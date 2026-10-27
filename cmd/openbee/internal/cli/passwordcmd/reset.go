package passwordcmd

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/spf13/cobra"

	"github.com/theopenbee/openbee/internal/infra/config"
	"github.com/theopenbee/openbee/internal/infra/i18n"
	"github.com/theopenbee/openbee/internal/infra/store"
)

const minPasswordLength = 6

func NewResetCommand() *cobra.Command {
	var (
		cfgPath  string
		password string
	)
	cmd := &cobra.Command{
		Use:   "reset-password",
		Short: i18n.M.Cmd.ResetPassword.Short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			err = resetPassword(cmd.OutOrStdout(), cfg.Database.Path, password, promptPassword)
			if errors.Is(err, terminal.InterruptErr) {
				fmt.Fprintln(cmd.OutOrStdout(), i18n.M.Prompt.Cancelled)
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", i18n.M.Flag.ConfigPath)
	cmd.Flags().StringVar(&password, "password", "", i18n.M.Flag.ResetPassword)
	return cmd
}

func resetPassword(out io.Writer, dbPath, password string, prompt func() (string, error)) error {
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf(i18n.M.Output.Reset.DBNotFound, dbPath)
	} else if err != nil {
		return fmt.Errorf("stat database: %w", err)
	}
	db, err := store.InitDB(dbPath)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	users := store.NewUserStore(db)
	id, found, err := users.AdminID()
	if err != nil {
		return fmt.Errorf("find admin: %w", err)
	}
	if !found {
		return errors.New(i18n.M.Output.Reset.NoAdmin)
	}
	admin, err := users.GetByID(id)
	if err != nil {
		return err
	}

	if password == "" {
		if password, err = prompt(); err != nil {
			return err
		}
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	if err := users.SetPassword(admin.ID, password); err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	fmt.Fprintf(out, i18n.M.Output.Reset.Done+"\n", admin.Username)
	return nil
}

func validatePassword(password string) error {
	if len([]rune(password)) < minPasswordLength {
		return fmt.Errorf(i18n.M.Validate.PasswordShort, minPasswordLength)
	}
	return nil
}

func promptPassword() (string, error) {
	var password, confirm string
	if err := survey.AskOne(&survey.Password{
		Message: i18n.M.Prompt.NewPassword,
	}, &password, survey.WithValidator(func(ans any) error {
		s, _ := ans.(string)
		return validatePassword(s)
	})); err != nil {
		return "", err
	}
	if err := survey.AskOne(&survey.Password{
		Message: i18n.M.Prompt.ConfirmPassword,
	}, &confirm); err != nil {
		return "", err
	}
	if confirm != password {
		return "", errors.New(i18n.M.Validate.PasswordMatch)
	}
	return password, nil
}
