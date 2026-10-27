package configcmd

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/AlecAivazis/survey/v2"
	"github.com/theopenbee/openbee/internal/infra/config"
)

// runAdvancedPrompts walks the advanced configuration prompts and updates vals.
func runAdvancedPrompts(vals *configValues) error {
	if err := survey.AskOne(&survey.Input{
		Message: "Server port:",
		Default: vals.ServerPort,
	}, &vals.ServerPort, survey.WithValidator(func(val any) error {
		s, _ := val.(string)
		if _, err := strconv.Atoi(s); err != nil {
			return errors.New("port must be an integer")
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: "Server Host:",
		Default: vals.ServerHost,
	}, &vals.ServerHost); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Confirm{
		Message: "Debug mode?",
		Default: vals.Debug,
	}, &vals.Debug); err != nil {
		return handleSurveyErr(err)
	}

	if err := survey.AskOne(&survey.Input{
		Message: "Database path:",
		Default: vals.DBPath,
	}, &vals.DBPath); err != nil {
		return handleSurveyErr(err)
	}

	var concurrentBeeStr string
	if err := survey.AskOne(&survey.Input{
		Message: "Max concurrent bee processes:",
		Default: strconv.Itoa(vals.FeederMaxConcurrentBee),
	}, &concurrentBeeStr, survey.WithValidator(func(ans any) error {
		s, _ := ans.(string)
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			return errors.New("must be a positive integer")
		}
		return nil
	})); err != nil {
		return handleSurveyErr(err)
	}
	vals.FeederMaxConcurrentBee, _ = strconv.Atoi(concurrentBeeStr)

	if err := survey.AskOne(&survey.Input{
		Message: "Message debounce:",
		Default: vals.MessageDebounce,
	}, &vals.MessageDebounce); err != nil {
		return handleSurveyErr(err)
	}

	if err := maybeRegenSecret(&vals.AuthJWTSecret, "JWT secret already exists. Regenerate?", "JWT secret regenerated.", "JWT secret generated.", ""); err != nil {
		return err
	}
	return maybeRegenSecret(&vals.RPCTokenSecret, "RPC Token Secret already exists. Regenerate?", "RPC Token Secret regenerated.", "", "Generated RPC Token Secret: %s")
}

// maybeRegenSecret prompts for regeneration of an existing secret or generates
// one if currently empty. regeneratedMsg is printed after regenerating an
// existing secret. generatedMsg (with no %s) is printed when filling an empty
// slot without showing the value; generatedMsgWithValue (with %s) is printed
// instead when the new value should be revealed.
func maybeRegenSecret(secret *string, confirmMsg, regeneratedMsg, generatedMsg, generatedMsgWithValue string) error {
	if *secret == "" {
		*secret = config.GenerateRandomSecret()
		if generatedMsgWithValue != "" {
			fmt.Printf(generatedMsgWithValue+"\n", *secret)
		} else if generatedMsg != "" {
			fmt.Println(generatedMsg)
		}
		return nil
	}
	var regenerate bool
	if err := survey.AskOne(&survey.Confirm{
		Message: confirmMsg,
		Default: false,
	}, &regenerate); err != nil {
		return handleSurveyErr(err)
	}
	if regenerate {
		*secret = config.GenerateRandomSecret()
		fmt.Println(regeneratedMsg)
	}
	return nil
}
