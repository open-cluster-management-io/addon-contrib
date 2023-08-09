IMG ?= quay.io/skeeey/opcua-server
PROXY ?= http://squid.corp.redhat.com:3128

image:
	docker build --build-arg=HTTP_PROXY=${PROXY} --build-arg=HTTPS_PROXY=${PROXY} -t ${IMG} .
