# fusionlocalserver Makefile
#
# The APS client_id is injected at build time so it never appears in source.
# Store your client_id in a local .aps-client-id file (git-ignored), or set
# the CLIENT_ID variable directly:
#
#   make build CLIENT_ID=your-client-id
#   make install CLIENT_ID=your-client-id

CLIENT_ID  ?= $(shell cat .aps-client-id 2>/dev/null | tr -d '[:space:]')
REGION     ?= $(shell cat .aps-region 2>/dev/null | tr -d '[:space:]')
MODULE     := github.com/schneik80/fusionlocalserver
VERSION    ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS    := -X $(MODULE)/config.DefaultClientID=$(CLIENT_ID) \
              -X $(MODULE)/config.DefaultRegion=$(REGION) \
              -X main.version=$(VERSION)

.PHONY: build install run clean check web

# Build the React/MUI web UI into server/webdist. The whole server/webdist/
# tree is gitignored build output; the `-tags embed_ui` builds below embed it
# via server/static_embed.go. build and install depend on this so the binary
# always ships the current UI. Requires Node/npm.
web:
	cd web && npm install && npm run build

# Production build: bundle the UI and embed it (-tags embed_ui). Without the tag
# the binary serves the "not built yet" stub (server/static_stub.go) instead.
build: web
	@[ -n "$(CLIENT_ID)" ] || (echo "ERROR: CLIENT_ID is not set. See Makefile header." && exit 1)
	go build -tags embed_ui -ldflags "$(LDFLAGS)" -o fusionlocalserver .

install: web
	@[ -n "$(CLIENT_ID)" ] || (echo "ERROR: CLIENT_ID is not set. See Makefile header." && exit 1)
	go install -tags embed_ui -ldflags "$(LDFLAGS)" .

# Build the full app and serve it on the LAN. Binds 0.0.0.0:8080 by default
# (change the port from the web UI's Settings dialog); startup logs the
# reachable http://<lan-ip>:8080 URLs so you can open it from another machine.
# Pass ARGS to add flags, e.g. `make run ARGS="-v"`.
run: build
	./fusionlocalserver $(ARGS)

# Dev build: no embedded UI and no embedded client_id — for local dev using env
# vars or config.json. Go-only (stub UI); pair with `cd web && npm run dev` and
# run `./fusionlocalserver -dev` for HMR.
dev:
	go build -o fusionlocalserver .

clean:
	rm -f fusionlocalserver

check:
	go vet ./...
	go test -race ./...
