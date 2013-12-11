package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	c := make(chan os.Signal, 0x100)
	for i := 0; i < 100; i++ {
		signal.Notify(c, syscall.Signal(i))
	}
	for i := range c {
		fmt.Println(i)
	}
}
