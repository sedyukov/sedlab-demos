module github.com/sedyukov/sedlab-demos/demos/prices

go 1.23

require (
	github.com/sedyukov/sedlab-modules/modules/accounts/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/ai/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/marketplaces/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/notify/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/scraping/go v0.0.0
	github.com/sedyukov/sedlab-modules/modules/sheets-storage/go v0.0.0
)

// Private modules: checked out next to this demo (CI: actions/checkout path demos/prices/.modules-src;
// locally: symlink .modules-src -> sedlab-modules clone).
replace (
	github.com/sedyukov/sedlab-modules/modules/accounts/go => ./.modules-src/modules/accounts/go
	github.com/sedyukov/sedlab-modules/modules/ai/go => ./.modules-src/modules/ai/go
	github.com/sedyukov/sedlab-modules/modules/marketplaces/go => ./.modules-src/modules/marketplaces/go
	github.com/sedyukov/sedlab-modules/modules/notify/go => ./.modules-src/modules/notify/go
	github.com/sedyukov/sedlab-modules/modules/scraping/go => ./.modules-src/modules/scraping/go
	github.com/sedyukov/sedlab-modules/modules/sheets-storage/go => ./.modules-src/modules/sheets-storage/go
)
