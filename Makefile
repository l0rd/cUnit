# Set an output prefix, which is the local directory if not specified
GIT_BRANCH=$(shell git rev-parse --abbrev-ref HEAD)

.PHONY: all clean image binaries

all: clean binaries

clean:
		@echo "+ $@"
		@rm -rf bundles

image:
		@echo "+ $@"
		@docker build -t docker-unit-build:${GIT_BRANCH} .

binaries: image
		@echo "+ $@"
		$(eval C_ID := $(shell docker create docker-unit-build:${GIT_BRANCH}))
		@docker start -a ${C_ID}
		@docker cp ${C_ID}:/bundles .

test: image
		@echo "+ $@"
		$(eval DOCKER_PATH := $(shell which docker))
		@docker run -it --entrypoint make_tests.sh docker-unit-build:${GIT_BRANCH}
		#@docker run -it --entrypoint make_tests.sh -v /var/run/docker.sock:/var/run/docker.sock -v ${DOCKER_PATH}:/bin/docker docker-unit-build:${GIT_BRANCH}
