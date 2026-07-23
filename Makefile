.PHONY: all web designer server cli run test clean

all: web designer server cli

web:
	cd web && npm install && npm run build
	rsync -a --delete web/dist/ internal/server/dist/

designer:
	cd designer && corepack pnpm install && corepack pnpm run build
	rsync -a --delete designer/dist/ internal/server/designerdist/

server:
	go build -o bin/labl-server ./cmd/labl-server

cli:
	go build -o bin/labl ./cmd/labl

run: server
	./bin/labl-server

test:
	go vet ./...
	go test -race ./...

clean:
	rm -rf bin web/dist designer/dist
