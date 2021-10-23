package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shu-go/gli"
	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type trashCmd struct {
	IDs gli.StrList `cli:"id" help:"IDs shown by 'list' subcommand to be deleted"`

	Confirm bool `cli:"confirm,i" help:"confirm each deletion" default:"true"`

	Format string      `cli:"format,f" default:"{id} {subject} ({date})" help:"{id}, {subject}, {date}, {snippet}, {body}"`
	Sort   gli.StrList `cli:"sort" default:"-date,subject,id" help:"sort criteria that is a list of [id, subject, date, snippet] (- means descending order)"`
}

func (c trashCmd) Run(g globalCmd, args []string) error {
	if len(c.IDs) == 0 && len(args) == 0 {
		return errors.New("--id or args are required")
	}

	config, err := getConfig(g.Credentials, g.ClientID, g.ClientSecret)
	if err != nil {
		return xerrors.Errorf("failed to get config: %v", err)
	}

	/*client*/
	_, token, err := getClient(config, g.Token, g.AuthPort)
	if err != nil {
		return xerrors.Errorf("failed to connect services: %v", err)
	}

	ctx := context.Background()
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))

	// check for label Notes/pomera_sync
	var pomeraSync *gmail.Label
	{
		labelsService := gmail.NewUsersLabelsService(gmailService)
		resp, err := labelsService.List(g.UserID).Do()
		if err != nil {
			return err
		}
		for _, lbl := range resp.Labels {
			if lbl.Name == g.Label {
				pomeraSync = lbl
			}
		}
		if pomeraSync == nil {
			return fmt.Errorf("Label %q not found", g.Label)
		}
	}

	list := make([]listItem, 0, 4)

	// list messages
	{
		idset := make(map[string]struct{})
		for _, id := range c.IDs {
			idset[id] = struct{}{}
		}

		msgService := gmail.NewUsersMessagesService(gmailService)

		q := strings.Join(args, " ")
		if q != "" {
			resp, err := msgService.List(g.UserID).LabelIds(pomeraSync.Id).Q(q).Do()
			if err != nil {
				return err
			}
			for _, msg := range resp.Messages {
				idset[msg.Id] = struct{}{}
			}
		}

		for id := range idset {
			//m, err := msgService.Get(c.LoginID, msg.Id).Format("metadata").Do()
			m, err := msgService.Get(g.UserID, id).Format("full").Do()
			if err != nil {
				return err
			}

			content := c.Format

			if strings.Index(c.Format, "{id}") != -1 {
				content = strings.Replace(content, "{id}", id, -1)
			}

			if strings.Index(c.Format, "{subject}") != -1 {
				content = strings.Replace(content, "{subject}", getHeader(m.Payload.Headers, "Subject"), -1)
			}

			if strings.Index(c.Format, "{headers}") != -1 {
				content = strings.Replace(content, "{headers}", fmt.Sprintf("%#v", m.Payload.Headers), -1)
			}

			if strings.Index(c.Format, "{date}") != -1 {
				content = strings.Replace(content, "{date}", getHeader(m.Payload.Headers, "Date"), -1)
			}

			if strings.Index(c.Format, "{snippet}") != -1 {
				content = strings.Replace(content, "{snippet}", m.Snippet, -1)
			}

			if strings.Index(c.Format, "{body}") != -1 {
				decoded, err := base64.URLEncoding.DecodeString(m.Payload.Body.Data)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					break //continue
				}
				content = strings.Replace(content, "{body}", string(decoded), -1)
			}

			dt, err := time.Parse(time.RFC822Z, getHeader(m.Payload.Headers, "Date"))
			if err != nil {
				dt = time.Now()
			}
			list = append(list, listItem{
				Content: content + dt.String(),
				ID:      m.Id,
				Subject: getHeader(m.Payload.Headers, "Subject"),
				Date:    dt,
				Snippet: m.Snippet,
			})
		}

		sortListItems(list, c.Sort)
		for _, item := range list {
			fmt.Println(item.Content)

			if c.Confirm {
				var yesno string
				fmt.Print("delete? [y/N]")
				n, err := fmt.Scanln(&yesno)

				if err != nil || n == 0 || len(yesno) < 1 || strings.ToLower(yesno)[0] != 'y' {
					continue
				}
			}

			_, err = msgService.Trash(g.UserID, item.ID).Do()
			if err != nil {
				return err
			}
		}
	}

	return nil
}
