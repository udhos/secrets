package main

import (
	"os"
	"strconv"

	"github.com/rs/zerolog/log"
)

// envBool extracts boolean value from env var.
// It returns the provided defaultValue if the env var is empty.
// The value returned is also recorded in logs.
func envBool(name string, defaultValue bool) bool {
	str := os.Getenv(name)
	if str != "" {
		value, errConv := strconv.ParseBool(str)
		if errConv == nil {
			log.Info().Msgf("%s=[%s] using %s=%v default=%v", name, str, name, value, defaultValue)
			return value
		}
		log.Info().Msgf("bad %s=[%s]: error: %v", name, str, errConv)
	}
	log.Info().Msgf("%s=[%s] using %s=%v default=%v", name, str, name, defaultValue, defaultValue)
	return defaultValue
}
