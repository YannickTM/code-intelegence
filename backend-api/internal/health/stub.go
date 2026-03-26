package health

import "context"

// StubChecker always returns "skipped" for dependencies not yet wired.
type StubChecker struct {
	name string
}

// NewStubChecker creates a stub checker with the given dependency name.
func NewStubChecker(name string) *StubChecker {
	return &StubChecker{name: name}
}

func (c *StubChecker) Name() string { return c.name }

func (c *StubChecker) Check(_ context.Context) CheckResult {
	return CheckResult{Status: StatusSkipped}
}
