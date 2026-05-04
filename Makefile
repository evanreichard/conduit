build_local:
	go mod download
	rm -r ./build || true
	mkdir -p ./build

	env GOOS=linux GOARCH=amd64  go build -ldflags "-X reichard.io/conduit/config.version=`git describe --tags`" -o ./build/server_linux_amd64
	env GOOS=linux GOARCH=arm64  go build -ldflags "-X reichard.io/conduit/config.version=`git describe --tags`" -o ./build/server_linux_arm64
	env GOOS=darwin GOARCH=arm64 go build -ldflags "-X reichard.io/conduit/config.version=`git describe --tags`" -o ./build/server_darwin_arm64
	env GOOS=darwin GOARCH=amd64 go build -ldflags "-X reichard.io/conduit/config.version=`git describe --tags`" -o ./build/server_darwin_amd64

docker_build_local:
	docker build -t conduit:latest .

docker_build_release_dev:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t gitea.va.reichard.io/evan/conduit:dev \
		-f Dockerfile-BuildKit \
		--push .

docker_build_release_latest:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		-t gitea.va.reichard.io/evan/conduit:latest \
		-t gitea.va.reichard.io/evan/conduit:`git describe --tags` \
		-f Dockerfile-BuildKit \
		--push .

clean:
	rm -rf ./build

tests:
	SET_TEST=set_val go test -race -coverpkg=./... ./... -coverprofile=./cover.out
	go tool cover -html=./cover.out -o ./cover.html
	rm ./cover.out
