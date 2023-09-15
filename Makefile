BUILD_FLAGS = -tags "$(BUILD_TAGS)" -ldflags "

build:
	@ echo "Building Neatio Client..."
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building."
#.PHONY: neatio
neatio:
	@ echo "Building Neatio Client..."
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building."
	@ echo "Run ./neatio to launch the client. "

install:
	@ echo "Installing..."
	@ go install -mod=readonly $(BUILD_FLAGS) ./chain/neatio
	@ echo "Install success."