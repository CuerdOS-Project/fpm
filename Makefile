CC      ?= cc
CFLAGS  ?= -std=c11 -Wall -Wextra -O2
LDFLAGS ?= -lncurses

SRC_DIR := src
BUILD_DIR := build
BIN := fpm

SOURCES := $(wildcard $(SRC_DIR)/*.c)
OBJECTS := $(SOURCES:$(SRC_DIR)/%.c=$(BUILD_DIR)/%.o)

PREFIX      ?= /usr/local
BINDIR      := $(PREFIX)/bin
LOCALE_DIR  := $(PREFIX)/share/fpm/locales

.PHONY: all clean install uninstall

all: $(BIN)

$(BIN): $(OBJECTS)
	$(CC) $(CFLAGS) -o $@ $(OBJECTS) $(LDFLAGS)

$(BUILD_DIR)/%.o: $(SRC_DIR)/%.c | $(BUILD_DIR)
	$(CC) $(CFLAGS) -c $< -o $@

$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)

clean:
	rm -rf $(BUILD_DIR) $(BIN)

install: all
	install -Dm755 $(BIN) $(DESTDIR)$(BINDIR)/$(BIN)
	install -d $(DESTDIR)$(LOCALE_DIR)
	install -Dm644 locales/*.kn -t $(DESTDIR)$(LOCALE_DIR)

uninstall:
	rm -f $(DESTDIR)$(BINDIR)/$(BIN)
	rm -rf $(DESTDIR)$(LOCALE_DIR)
