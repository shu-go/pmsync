package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
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
	if err != nil {
		return xerrors.Errorf("failed to instantiate a gmail service: %v", err)
	}

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

		wg := sync.WaitGroup{}
		mut := sync.Mutex{}

		for id := range idset {
			wg.Add(1)
			go func(id string) {
				//m, err := msgService.Get(c.LoginID, msg.Id).Format("metadata").Do()
				m, err := msgService.Get(g.UserID, id).Format("full").Do()
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					return
				}

				content := c.Format
				content = strings.ReplaceAll(content, "{id}", id)
				if strings.Contains(content, "{subject}") {
					content = strings.ReplaceAll(content, "{subject}", getHeader(m.Payload.Headers, "Subject"))
				}
				if strings.Contains(content, "{headers}") {
					content = strings.ReplaceAll(content, "{headers}", fmt.Sprintf("%#v", m.Payload.Headers))
				}
				if strings.Contains(content, "{date}") {
					content = strings.ReplaceAll(content, "{date}", getHeader(m.Payload.Headers, "Date"))
				}
				if strings.Contains(content, "{snippet}") {
					content = strings.ReplaceAll(content, "{snippet}", m.Snippet)
				}
				if strings.Contains(c.Format, "{body}") {
					decoded, err := base64.URLEncoding.DecodeString(m.Payload.Body.Data)
					if err != nil {
						fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
						return
					}
					content = strings.ReplaceAll(content, "{body}", string(decoded))
				}

				dt, err := time.Parse(time.RFC822Z, getHeader(m.Payload.Headers, "Date"))
				if err != nil {
					dt = time.Now()
				}

				mut.Lock()
				list = append(list, listItem{
					Content: content,
					ID:      m.Id,
					Subject: getHeader(m.Payload.Headers, "Subject"),
					Date:    dt,
					Snippet: m.Snippet,
				})
				mut.Unlock()

				wg.Done()
			}(id)
		}
		wg.Wait()

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
