BUILD_FLAGS = -tags "$(BUILD_TAGS)" -ldflags "

build:
	@ echo "Building Neatio full node..."
	@ go build -o $(GOPATH)/bin/neatio ./cmd/neatio/
	@ echo "Done building!"
neatio:
	@ echo "Building Neatio full node..."
	@ go build -o $(GOPATH)/bin/neatio ./cmd/neatio/
	@ echo "Done building!"
	@ echo "Run 'neatio' to start Neatio full node."

install:
	@ echo "Installing..."
	@ go install -mod=readonly $(BUILD_FLAGS) ./cmd/neatio
	@ echo "Installation done!"