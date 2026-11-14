package main

import (
	"os"

	"example.com/fpm/internal/fpm"
)

func main() {
	os.Exit(fpm.Run())
}
