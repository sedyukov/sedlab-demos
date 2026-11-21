module github.com/sedyukov/sedlab-demos/demos/leads

go 1.23

require (
	github.com/sedyukov/sedlab-modules/modules/accounts/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/crm/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/notify/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/payments/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/sheets-storage/go v0.0.0
)

// Private modules: checked out next to this demo (CI: actions/checkout path demos/leads/.modules-src;
// locally: symlink .modules-src -> sedlab-modules clone).
replace (
	github.com/sedyukov/sedlab-modules/modules/accounts/go => ./.modules-src/modules/accounts/go
	github.com/sedyukov/sedlab-modules/modules/crm/go => ./.modules-src/modules/crm/go
	github.com/sedyukov/sedlab-modules/modules/notify/go => ./.modules-src/modules/notify/go
	github.com/sedyukov/sedlab-modules/modules/payments/go => ./.modules-src/modules/payments/go
	github.com/sedyukov/sedlab-modules/modules/sheets-storage/go => ./.modules-src/modules/sheets-storage/go
)
