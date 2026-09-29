PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin

.PHONY: all build test vet race install uninstall clean check-i18n

all: build

build:
	go build -trimpath -ldflags "-s -w" -o fpm .

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check-i18n:
	./check-i18n.sh

install: build
	install -Dm755 fpm $(DESTDIR)$(BINDIR)/fpm

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/fpm

clean:
	rm -f fpm
