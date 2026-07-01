.PHONY: build docker-build clean

IMAGE ?= nantian-echo:latest

build:
	go build -o echo-server .

docker-build:
	docker build -t $(IMAGE) .

clean:
	rm -f echo-server
