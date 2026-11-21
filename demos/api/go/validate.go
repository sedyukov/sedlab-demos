package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// parseObject: the body must be a JSON object (BEHAVIOR.md "Validation").
func parseObject(body []byte) (map[string]any, *apiError) {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, validationErr([]detail{{"body", "invalid_json"}})
	}
	if _, err := dec.Token(); err != io.EOF { // trailing data
		return nil, validationErr([]detail{{"body", "invalid_json"}})
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, validationErr([]detail{{"body", "invalid_type"}})
	}
	return obj, nil
}

// validator walks fields in schema order and collects every problem.
type validator struct {
	obj     map[string]any
	patch   bool
	seen    map[string]bool
	out     map[string]any // validated values (string, int64, []string, nil)
	details []detail
}

func newValidator(obj map[string]any, patch bool) *validator {
	return &validator{obj: obj, patch: patch, seen: map[string]bool{}, out: map[string]any{}}
}

func (v *validator) add(field, issue string) { v.details = append(v.details, detail{field, issue}) }

// get returns (value, present). Records "required" when absent on create.
func (v *validator) get(name string, required bool) (any, bool) {
	v.seen[name] = true
	val, ok := v.obj[name]
	if !ok && required && !v.patch {
		v.add(name, "required")
	}
	return val, ok
}

func (v *validator) null(name string, val any, nullable bool) bool {
	if val != nil {
		return false
	}
	if nullable {
		v.out[name] = nil
	} else {
		v.add(name, "invalid_type")
	}
	return true
}

func (v *validator) str(name string, required, nullable bool, min, max int) {
	val, ok := v.get(name, required)
	if !ok || v.null(name, val, nullable) {
		return
	}
	s, isStr := val.(string)
	if !isStr {
		v.add(name, "invalid_type")
		return
	}
	n := utf8.RuneCountInString(s)
	switch {
	case n < min:
		v.add(name, "too_short")
	case n > max:
		v.add(name, "too_long")
	default:
		v.out[name] = s
	}
}

func (v *validator) integer(name string, required bool, min, max int64) {
	val, ok := v.get(name, required)
	if !ok || v.null(name, val, false) {
		return
	}
	num, isNum := val.(json.Number)
	if !isNum || strings.ContainsAny(num.String(), ".eE") {
		v.add(name, "invalid_type")
		return
	}
	i, err := strconv.ParseInt(num.String(), 10, 64)
	if err != nil || i < min || i > max {
		v.add(name, "out_of_range")
		return
	}
	v.out[name] = i
}

func (v *validator) enum(name string, required bool, allowed ...string) {
	val, ok := v.get(name, required)
	if !ok || v.null(name, val, false) {
		return
	}
	s, isStr := val.(string)
	if !isStr {
		v.add(name, "invalid_type")
		return
	}
	for _, a := range allowed {
		if s == a {
			v.out[name] = s
			return
		}
	}
	v.add(name, "invalid")
}

var dateRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

func validDate(s string) bool {
	if !dateRe.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (v *validator) date(name string, nullable bool) {
	val, ok := v.get(name, false)
	if !ok || v.null(name, val, nullable) {
		return
	}
	s, isStr := val.(string)
	if !isStr {
		v.add(name, "invalid_type")
		return
	}
	if !validDate(s) {
		v.add(name, "invalid")
		return
	}
	v.out[name] = s
}

var tagRe = regexp.MustCompile(`^[a-z0-9-]{1,24}$`)

func (v *validator) tags(name string) {
	val, ok := v.get(name, false)
	if !ok || v.null(name, val, false) {
		return
	}
	arr, isArr := val.([]any)
	if !isArr {
		v.add(name, "invalid_type")
		return
	}
	if len(arr) > 5 {
		v.add(name, "too_long")
		return
	}
	out := []string{}
	seen := map[string]bool{}
	bad := false
	for i, it := range arr {
		f := name + "[" + strconv.Itoa(i) + "]"
		s, isStr := it.(string)
		if !isStr {
			v.add(f, "invalid_type")
			bad = true
			continue
		}
		if !tagRe.MatchString(s) {
			v.add(f, "invalid")
			bad = true
			continue
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	if !bad {
		v.out[name] = out
	}
}

// unknown reports fields outside the schema (alphabetical) and the empty PATCH body.
func (v *validator) unknown() {
	var extra []string
	for k := range v.obj {
		if !v.seen[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(extra)
	for _, k := range extra {
		v.add(k, "unknown")
	}
}

// ---- query parameters ----

var intRe = regexp.MustCompile(`^-?[0-9]+$`)

type queryV struct {
	q       url.Values
	details []detail
}

func (qv *queryV) add(field, issue string) { qv.details = append(qv.details, detail{field, issue}) }

func (qv *queryV) has(name string) bool { return qv.q.Has(name) }

func (qv *queryV) str(name string) (string, bool) {
	if !qv.q.Has(name) {
		return "", false
	}
	return qv.q.Get(name), true
}

func (qv *queryV) intParam(name string, min, max int64, def int64) int64 {
	s, ok := qv.str(name)
	if !ok {
		return def
	}
	if !intRe.MatchString(s) {
		qv.add(name, "invalid_type")
		return def
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil || i < min || i > max {
		qv.add(name, "out_of_range")
		return def
	}
	return i
}

func (qv *queryV) search(name string) string {
	s, ok := qv.str(name)
	if !ok {
		return ""
	}
	if utf8.RuneCountInString(s) > 100 {
		qv.add(name, "too_long")
		return ""
	}
	return strings.ToLower(s)
}

func (qv *queryV) date(name string) string {
	s, ok := qv.str(name)
	if !ok {
		return ""
	}
	if !validDate(s) {
		qv.add(name, "invalid")
		return ""
	}
	return s
}

func (qv *queryV) enum(name, def string, allowed ...string) string {
	s, ok := qv.str(name)
	if !ok {
		return def
	}
	for _, a := range allowed {
		if s == a {
			return s
		}
	}
	qv.add(name, "invalid")
	return def
}
