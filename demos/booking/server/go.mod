module github.com/sedyukov/sedlab-demos/demos/booking/server

go 1.26.0

require (
	github.com/sedyukov/sedlab-modules/modules/max-core/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/notify/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/payments/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/telegram-core/go v0.0.0
	modernc.org/sqlite v1.62.0
)

require (
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/sys v0.49.0 // indirect
	modernc.org/libc v1.80.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

// Private modules: CI/Docker check them out into <repo>/modules-src (see .github/workflows/build-booking.yml).
replace (
	github.com/sedyukov/sedlab-modules/modules/max-core/go => ../../../modules-src/modules/max-core/go
	github.com/sedyukov/sedlab-modules/modules/notify/go => ../../../modules-src/modules/notify/go
	github.com/sedyukov/sedlab-modules/modules/payments/go => ../../../modules-src/modules/payments/go
	github.com/sedyukov/sedlab-modules/modules/telegram-core/go => ../../../modules-src/modules/telegram-core/go
)
