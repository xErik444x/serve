.PHONY: build build-all test bench clean install

# make build — binario local
build:
	go build -o serve .

# make build-all — cross-compile para Windows/macOS/Linux (Release)
build-all:
	GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/serve-windows-amd64.exe .
	GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/serve-darwin-arm64 .
	GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/serve-linux-amd64 .
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/serve-linux-arm64 .
	@ls -la dist/

# make test
test:
	go test -v -count=1 ./...

# make bench — benchmark vs npx serve (requiere npx)
bench:
	go build -o loadtest ./cmd/loadtest
	@echo "levantá serve-go y npx serve en :4510/:4511 y corré:"
	@echo "  ./loadtest -n 2000 -c 50 http://127.0.0.1:4510/page1.html"
	@echo "  ./loadtest -n 2000 -c 50 http://127.0.0.1:4511/page1.html"

# make install — binario al PATH
install: build
	cp serve /usr/local/bin/serve

clean:
	rm -rf dist serve loadtest
