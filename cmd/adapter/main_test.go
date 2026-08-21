package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDryRunLogOptionsDefaults(t *testing.T) {
	level, format := buildDryRunLogOptions()

	require.Equal(t, "warn", level)
	require.Equal(t, "text", format)
}

func TestDryRunLogOptionsHonorsLevelOverride(t *testing.T) {
	t.Setenv("LOG_LEVEL", "debug")

	level, _ := buildDryRunLogOptions()

	require.Equal(t, "debug", level)
}
