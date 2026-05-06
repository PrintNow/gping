package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func stdoutANSI() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("CLICOLOR") == "0" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// 使用“标准 16 色”而非固定 RGB：多数终端在浅色/深色主题下会重映射，比硬编码亮色更耐看。
const (
	ansiBold  = "\x1b[1m"
	ansiBlue  = "\x1b[34m" // hostname
	ansiCyan  = "\x1b[36m" // IP
	ansiMag   = "\x1b[35m" // 地理位置
	ansiReset = "\x1b[0m"
)

func printGPINGLine(target, targetIP, location string) {
	if !stdoutANSI() {
		fmt.Printf("GPING %s (%s): %s\n", target, targetIP, location)
		return
	}
	fmt.Printf("%sGPING%s %s%s%s (%s%s%s): %s%s%s\n",
		ansiBold, ansiReset,
		ansiBlue, target, ansiReset,
		ansiCyan, targetIP, ansiReset,
		ansiMag, location, ansiReset,
	)
}
