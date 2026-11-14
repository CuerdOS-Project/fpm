PREFIX ?= /usr/local
BINDIR := $(PREFIX)/bin
BASHCOMPDIR ?= $(PREFIX)/share/bash-completion/completions
ZSHCOMPDIR  ?= $(PREFIX)/share/zsh/site-functions
FISHCOMPDIR ?= $(PREFIX)/share/fish/vendor_completions.d

.PHONY: all build test vet race install uninstall clean check-i18n

all: build

build:
	go build -trimpath -ldflags "-s -w" -o fpm ./cmd/fpm

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
	install -d $(DESTDIR)$(BASHCOMPDIR) $(DESTDIR)$(ZSHCOMPDIR) $(DESTDIR)$(FISHCOMPDIR)
	./fpm completion bash > $(DESTDIR)$(BASHCOMPDIR)/fpm
	./fpm completion zsh  > $(DESTDIR)$(ZSHCOMPDIR)/_fpm
	./fpm completion fish > $(DESTDIR)$(FISHCOMPDIR)/fpm.fish

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/fpm $(DESTDIR)$(BASHCOMPDIR)/fpm \
		$(DESTDIR)$(ZSHCOMPDIR)/_fpm $(DESTDIR)$(FISHCOMPDIR)/fpm.fish

clean:
	rm -f fpm
