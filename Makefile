BUILD_FLAGS = -tags "$(BUILD_TAGS)" -ldflags "

build:
	@ echo "Building Neatio Client..."
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building neatio client."
#.PHONY: neatio
neatio:
	@ echo "Building Neatio Client..."
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building."
	@ echo "Run ./neatio to launch neatio app. "

install:
	@ echo "Installing..."
	@ go install -mod=readonly $(BUILD_FLAGS) ./chain/neatio
	@ echo "Install success."