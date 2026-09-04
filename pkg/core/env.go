package core

import (
	"fmt"
	"strings"
)

type Environment int

const (
	Development Environment = iota + 1
	Staging
	Production
)

// String implements the Stringer interface
func (e Environment) String() string {
	if e < Development || e > Production {
		return "unknown"
	}

	return [...]string{"dev", "stage", "prod"}[e-1]
}

// Index makes the index of the Environment retrievable
func (e Environment) Index() int {
	return int(e)
}

// IsProduction is a utility function to quickly determine the environment
func (e Environment) IsProduction() bool {
	return e == Production
}

// IsStaging is a utility function to quickly determine the environment
func (e Environment) IsStaging() bool {
	return e == Staging
}

// IsDevelopment is a utility function to quickly determine the environment
func (e Environment) IsDevelopment() bool {
	return e == Development
}

// EnvFromString matches a string value for a known Environment returns the
// index (int) value for the Environment. If an invalid Environment is given,
// it returns Development along is a non-nil error
func EnvFromString(s string) (Environment, error) {
	switch strings.ToLower(s) {
	case Production.String():
		return Production, nil
	case Staging.String():
		return Staging, nil
	case Development.String():
		return Development, nil
	default:
		return Development, fmt.Errorf("invalid environment: %s", s)
	}
}
