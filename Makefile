secret:
	openssl rand -base64 48

build:
	CGO_ENABLED=0 go build -tags=grpcnotrace -trimpath -ldflags="-s -w -X 'qsp_acb_broker/version.version=$(VERSION)'" -o tyk_proxy ./cmd/tyk-proxy

test:
	go test ./... -v

gen:
	CGO_ENABLED=0 go build -tags=grpcnotrace -trimpath -ldflags="-s -w -X 'qsp_acb_broker/version.version=$(VERSION)'" -o token_gen ./cmd/token-gen
	./token_gen -secret "II+NZDtODCTp0eAGX0/3HNdaExOf+M1uesFHdN+IFcTD774aaeJrJIOMS4aYhi+l"

up:
	docker-compose up -d

up-b:
	docker-compose up -d --build --force-recreate