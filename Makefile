BUILD_FLAGS = -tags "$(BUILD_TAGS)" -ldflags "

build:
	@ echo "start building......"
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building."
#.PHONY: neatio
neatio:
	@ echo "start building......"
	@ go build -o $(GOPATH)/bin/neatio ./chain/neatio/
	@ echo "Done building."
	@ echo "Run neatio to launch neatio network."

install:
	@ echo "start install......"
	@ go install -mod=readonly $(BUILD_FLAGS) ./chain/neatio
	@ echo "install success......"