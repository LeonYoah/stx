swagger:
	scripts/swagger.sh

tidy:
	scripts/tidy.sh

check_license:
	scripts/license.sh

add_apache_license:
	python3 scripts/add_apache_license.py --git-added --staged

proto:
	scripts/proto.sh

pre_commit: tidy swagger check_license

# 先打包发版产物（dist/releases/），再构建只消费产物的 Docker 镜像。
# Package release artifacts (dist/releases/) first, then build images that only consume them.
# 可选变量 / Optional vars: DOCKER_PLATFORMS=linux/amd64,linux/arm64 DOCKER_IMAGES=all-in-one DOCKER_TAG=local
DOCKER_PLATFORMS ?=
DOCKER_IMAGES ?= backend,frontend,all-in-one
DOCKER_TAG ?= local
DOCKER_BUILD_ARGS = $(if $(DOCKER_PLATFORMS),--platforms $(DOCKER_PLATFORMS)) --images $(DOCKER_IMAGES) --tag $(DOCKER_TAG)

docker-images:
	scripts/docker-build-images.sh $(DOCKER_BUILD_ARGS) --verify

# 复用已有 dist/releases/，只重建镜像。/ Reuse existing dist/releases/ and rebuild images only.
docker-images-from-dist:
	scripts/docker-build-images.sh $(DOCKER_BUILD_ARGS) --skip-package --verify

# 复用已有 dist/releases/ 构建并对 all-in-one 跑 smoke。/ Build from existing artifacts and smoke-test all-in-one.
docker-smoke:
	scripts/docker-build-images.sh $(DOCKER_BUILD_ARGS) --skip-package --verify --smoke

.PHONY: docker-images docker-images-from-dist docker-smoke
