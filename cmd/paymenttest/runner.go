package main

import (
	"fmt"
	"os"
	"time"
)

const (
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
)

type Runner struct {
	total, failures, warnings, skipped int
	section                            string
	failed                             []string
	warned                             []string
}

func (r *Runner) Section(title string) {
	r.section = title
	fmt.Printf("\n%s== %s ==%s\n", colorBold, title, colorReset)
}

func (r *Runner) run(fn func() (string, error)) (string, error, time.Duration) {
	start := time.Now()
	detail, err := fn()
	return detail, err, time.Since(start).Round(time.Millisecond)
}

// Check runs a step; a failure is recorded and the run continues.
func (r *Runner) Check(name string, fn func() (string, error)) bool {
	r.total++
	detail, err, elapsed := r.run(fn)
	if err == nil {
		fmt.Printf("%s[PASS]%s %-62s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed)
		return true
	}
	r.failures++
	r.failed = append(r.failed, r.section+" › "+name+": "+err.Error())
	fmt.Printf("%s[FAIL]%s %-62s %v (%s)\n", colorRed, colorReset, name, err, elapsed)
	return false
}

// Warn is for steps whose outcome depends on Stripe timing or on a business
// decision rather than a defect; reported, never fatal.
func (r *Runner) Warn(name string, fn func() (string, error)) bool {
	r.total++
	detail, err, elapsed := r.run(fn)
	if err == nil {
		fmt.Printf("%s[PASS]%s %-62s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed)
		return true
	}
	r.warnings++
	r.warned = append(r.warned, r.section+" › "+name+": "+err.Error())
	fmt.Printf("%s[WARN]%s %-62s %v (%s)\n", colorYellow, colorReset, name, err, elapsed)
	return false
}

// MustCheck aborts the run on failure, for steps everything later depends on.
func (r *Runner) MustCheck(name string, fn func() (string, error)) {
	if !r.Check(name, fn) {
		r.Summary()
		fmt.Println("\naborting: this step is required for everything that follows")
		os.Exit(1)
	}
}

func (r *Runner) Skip(name, reason string) {
	r.skipped++
	fmt.Printf("%s[SKIP]%s %-62s %s\n", colorCyan, colorReset, name, reason)
}

func (r *Runner) Summary() {
	passed := r.total - r.failures - r.warnings
	fmt.Printf("\n%s%d checks: %s%d passed%s, %s%d failed%s, %s%d warnings%s, %s%d skipped%s%s\n",
		colorBold, r.total,
		colorGreen, passed, colorReset,
		colorRed, r.failures, colorReset,
		colorYellow, r.warnings, colorReset,
		colorCyan, r.skipped, colorReset,
		colorReset)
	if len(r.failed) > 0 {
		fmt.Printf("\n%sFailures:%s\n", colorRed, colorReset)
		for _, f := range r.failed {
			fmt.Println("  - " + f)
		}
	}
	if len(r.warned) > 0 {
		fmt.Printf("\n%sWarnings:%s\n", colorYellow, colorReset)
		for _, w := range r.warned {
			fmt.Println("  - " + w)
		}
	}
}

func (r *Runner) ExitCode() int {
	if r.failures > 0 {
		return 1
	}
	return 0
}
