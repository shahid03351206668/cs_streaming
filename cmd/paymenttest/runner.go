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
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
)

type Runner struct {
	failures int
	warnings int
	total    int
}

// Check runs a named step. fn returns a human-readable detail string and an
// error. A nil error is a pass; an error marked with warnOnly continues the
// run but is still reported at the end.
func (r *Runner) Check(name string, fn func() (string, error)) {
	r.total++
	start := time.Now()
	detail, err := fn()
	elapsed := time.Since(start)

	if err == nil {
		fmt.Printf("%s[PASS]%s %-55s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed.Round(time.Millisecond))
		return
	}

	r.failures++
	fmt.Printf("%s[FAIL]%s %-55s %v (%s)\n", colorRed, colorReset, name, err, elapsed.Round(time.Millisecond))
}

// Warn runs a step whose failure should not stop the suite (e.g. things that
// are expected to be blocked, like withdrawing unavailable Stripe balance).
func (r *Runner) Warn(name string, fn func() (string, error)) {
	r.total++
	start := time.Now()
	detail, err := fn()
	elapsed := time.Since(start)

	if err == nil {
		fmt.Printf("%s[PASS]%s %-55s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed.Round(time.Millisecond))
		return
	}

	r.warnings++
	fmt.Printf("%s[WARN]%s %-55s %v (%s)\n", colorYellow, colorReset, name, err, elapsed.Round(time.Millisecond))
}

// MustCheck is like Check but aborts the whole run immediately on failure,
// for steps every later step depends on (e.g. registering the test users).
func (r *Runner) MustCheck(name string, fn func() (string, error)) {
	r.total++
	start := time.Now()
	detail, err := fn()
	elapsed := time.Since(start)

	if err == nil {
		fmt.Printf("%s[PASS]%s %-55s %s (%s)\n", colorGreen, colorReset, name, detail, elapsed.Round(time.Millisecond))
		return
	}

	r.failures++
	fmt.Printf("%s[FAIL]%s %-55s %v (%s)\n", colorRed, colorReset, name, err, elapsed.Round(time.Millisecond))
	r.Summary()
	fmt.Println("\naborting: this step is required for everything that follows")
	os.Exit(1)
}

func (r *Runner) Section(title string) {
	fmt.Printf("\n%s== %s ==%s\n", colorBold, title, colorReset)
}

func (r *Runner) Summary() {
	passed := r.total - r.failures - r.warnings
	fmt.Printf("\n%s%d checks: %s%d passed%s, %s%d failed%s, %s%d warnings%s%s\n",
		colorBold, r.total,
		colorGreen, passed, colorReset,
		colorRed, r.failures, colorReset,
		colorYellow, r.warnings, colorReset,
		colorReset,
	)
}

func (r *Runner) ExitCode() int {
	if r.failures > 0 {
		return 1
	}
	return 0
}
