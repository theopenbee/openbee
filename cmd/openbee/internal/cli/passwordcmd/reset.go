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
		Short: "Reset the web console admin password",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(cfgPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			err = resetPassword(cmd.OutOrStdout(), cfg.Database.Path, password, promptPassword)
			if errors.Is(err, terminal.InterruptErr) {
				fmt.Fprintln(cmd.OutOrStdout(), "\nCancelled.")
				return nil
			}
			return err
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", "config.yaml", "path to config file")
	cmd.Flags().StringVar(&password, "password", "", "new password (prompted when omitted)")
	return cmd
}

func resetPassword(out io.Writer, dbPath, password string, prompt func() (string, error)) error {
	if _, err := os.Stat(dbPath); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("Database not found at %s. Check database.path in the config file.", dbPath)
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
		return errors.New("No admin account yet. Open the web console to finish the first-run setup.")
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
	fmt.Fprintf(out, "Password reset for admin %q. Signed-in sessions must log in again.\n", admin.Username)
	return nil
}

func validatePassword(password string) error {
	if len([]rune(password)) < minPasswordLength {
		return fmt.Errorf("password must be at least %d characters", minPasswordLength)
	}
	return nil
}

func promptPassword() (string, error) {
	var password, confirm string
	if err := survey.AskOne(&survey.Password{
		Message: "New password:",
	}, &password, survey.WithValidator(func(ans any) error {
		s, _ := ans.(string)
		return validatePassword(s)
	})); err != nil {
		return "", err
	}
	if err := survey.AskOne(&survey.Password{
		Message: "Confirm new password:",
	}, &confirm); err != nil {
		return "", err
	}
	if confirm != password {
		return "", errors.New("passwords do not match")
	}
	return password, nil
}
