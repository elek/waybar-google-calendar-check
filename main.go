package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/zeebo/errs/v2"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path"
	"sort"
	"strings"
	"time"
)

func main() {
	cmd := cobra.Command{}
	configDir := cmd.PersistentFlags().String("config-dir", "${HOME}/.config/waybar-google-calendar-check", "Directory to store the tokens (and credentials)")
	{
		subCmd := cobra.Command{
			Use:   "run",
			Short: "Check gmail inbox and return the unread information in waybar format.",
		}
		calendar := subCmd.Flags().String("calendar", "", "Identifier of the calendar (use list to print out available options")
		subCmd.RunE = func(cmd *cobra.Command, args []string) error {
			return run(getConfigDir(*configDir), *calendar)
		}
		cmd.AddCommand(&subCmd)
	}
	{
		subCmd := cobra.Command{
			Use:   "setup",
			Short: "Setup credentials",
		}
		subCmd.RunE = func(cmd *cobra.Command, args []string) error {
			return setup(getConfigDir(*configDir))
		}
		cmd.AddCommand(&subCmd)
	}
	{
		subCmd := cobra.Command{
			Use:   "list",
			Short: "List available calendars",
		}
		subCmd.RunE = func(cmd *cobra.Command, args []string) error {
			return list(getConfigDir(*configDir))
		}
		cmd.AddCommand(&subCmd)
	}
	err := cmd.Execute()
	if err != nil {
		log.Fatalf("%++v", err)
	}
}

func getConfigDir(dir string) string {
	user, err := user.Current()
	if err != nil {
		return dir
	}
	return strings.ReplaceAll(dir, "${HOME}", user.HomeDir)
}

func setup(configDir string) (err error) {
	config, err := readCredentials(configDir)
	if err != nil {
		return errs.Wrap(err)
	}

	ctx := context.Background()
	token, _ := readToken(configDir)

	token.Expiry = time.Now().Add(-time.Hour)

	if !token.Valid() {
		if token.RefreshToken != "" {
			token, err = config.TokenSource(ctx, token).Token()
			if err != nil {
				fmt.Println(err)
			}
		}
		if !token.Valid() {
			fmt.Println(config.AuthCodeURL("no-state", oauth2.AccessTypeOffline))
			var authCode string
			if _, err := fmt.Scan(&authCode); err != nil {
				return errs.Wrap(err)
			}
			token, err := config.Exchange(ctx, authCode)
			if err != nil {
				if _, err := fmt.Scan(&authCode); err != nil {
					return errs.Wrap(err)
				}
			}
			tokenBytes, err := json.Marshal(token)
			if err != nil {
				return errs.Wrap(err)
			}
			err = ioutil.WriteFile(path.Join(configDir, "token.json"), tokenBytes, 0600)
			if err != nil {
				return errs.Wrap(err)
			}
		}

	}
	return nil

}

func list(configDir string) error {
	ctx := context.Background()

	config, err := readCredentials(configDir)
	if err != nil {
		return errs.Wrap(err)
	}
	token, err := readToken(configDir)
	if err != nil {
		return err
	}

	service, err := calendar.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		return errs.Wrap(err)
	}
	calendars, err := service.CalendarList.List().Do()
	if err != nil {
		return errs.Wrap(err)
	}
	for _, cal := range calendars.Items {
		fmt.Printf("%s %s\n", cal.Id, cal.Description)
	}
	return nil
}

type Event struct {
	start time.Time
	raw   *calendar.Event
}

func run(configDir string, id string) (err error) {
	ctx := context.Background()

	config, err := readCredentials(configDir)
	if err != nil {
		return errs.Wrap(err)
	}
	token, err := readToken(configDir)
	if err != nil {
		return err
	}

	service, err := calendar.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))
	if err != nil {
		return errs.Wrap(err)
	}

	from := time.Now().Truncate(time.Hour * 24)
	to := from.Add(time.Hour * 24)
	events, err := service.Events.List(id).TimeMin(from.Format(time.RFC3339)).SingleEvents(true).TimeMax(to.Format(time.RFC3339)).Do()
	if err != nil {
		return errs.Wrap(err)
	}

	sort.Slice(events.Items, func(i, j int) bool {
		start1, _ := time.Parse(time.RFC3339, events.Items[i].Start.DateTime)
		start2, _ := time.Parse(time.RFC3339, events.Items[j].Start.DateTime)
		return start1.Before(start2)
	})

	jsonOutput := json.NewEncoder(os.Stdout)
	if len(events.Items) == 0 {
		return jsonOutput.Encode(BarItem{
			Text: "",
		})
	}

	var next *calendar.Event
	alt := ""
	for i := 0; i < len(events.Items); i++ {
		start, _ := time.Parse(time.RFC3339, events.Items[i].Start.DateTime)
		if next == nil && time.Now().Before(start.Add(5*time.Minute)) {
			next = events.Items[i]
		}
		alt += fmt.Sprintf("%s %s\n", start.Format("15:04"), events.Items[i].Summary)

	}

	start, _ := time.Parse(time.RFC3339, next.Start.DateTime)
	return jsonOutput.Encode(BarItem{
		Text:    fmt.Sprintf("%s %s", start.Format("15:04"), next.Summary),
		Tooltip: alt,
	})
}
9
func readToken(dir string) (*oauth2.Token, error) {
	t := &oauth2.Token{}
	content, err := ioutil.ReadFile(path.Join(dir, "token.json"))
	if err != nil {
		return t, errs.Wrap(err)
	}
	err = json.Unmarshal(content, t)
	if err != nil {
		return t, errs.Wrap(err)
	}
	return t, nil
}

func readCredentials(configDir string) (*oauth2.Config, error) {
	credentialFile := path.Join(configDir, "credentials.json")
	content, err := ioutil.ReadFile(credentialFile)
	if err != nil {
		return nil, errs.Errorf("Couldn't read credentials file from %s: %v", credentialFile, err)
	}

	config, err := google.ConfigFromJSON(content, calendar.CalendarReadonlyScope)
	if err != nil {
		log.Fatalf("Couldn't parse configuration: %v", err)
	}
	return config, nil
}
