package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/shu-go/gli"
	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type listCmd struct {
	Format string      `cli:"format,f" default:"{id} {subject} ({date})" help:"{id}, {subject}, {date}, {snippet}, {body}"`
	Sort   gli.StrList `cli:"sort" default:"-date,subject,id" help:"sort criteria that is a list of [id, subject, date, snippet] (- means descending order)"`
}

func (c listCmd) Run(g globalCmd, args []string) error {
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

	q := strings.Join(args, " ")

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
		msgService := gmail.NewUsersMessagesService(gmailService)
		resp, err := msgService.List(g.UserID).LabelIds(pomeraSync.Id).Q(q).Do()
		if err != nil {
			return err
		}
		for _, msg := range resp.Messages {
			//m, err := msgService.Get(c.LoginID, msg.Id).Format("metadata").Do()
			m, err := msgService.Get(g.UserID, msg.Id).Format("full").Do()
			if err != nil {
				return err
			}

			content := c.Format

			if strings.Index(c.Format, "{id}") != -1 {
				content = strings.Replace(content, "{id}", m.Id, -1)
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
	}

	sortListItems(list, c.Sort)
	for _, item := range list {
		fmt.Println(item.Content)
	}

	return nil
}

// used to collect displaying items
// list, trash
type listItem struct {
	Content string

	ID, Subject, Snippet string
	Date                 time.Time
}

func sortListItems(list []listItem, criteria []string) {
	sort.Slice(list, func(i, j int) bool {
		for _, c := range criteria {
			switch strings.ToLower(c) {
			case "id":
				if list[i].ID < list[j].ID {
					return true
				} else if list[i].ID > list[j].ID {
					return false
				}
			case "-id":
				if list[j].ID < list[i].ID {
					return true
				} else if list[j].ID > list[i].ID {
					return false
				}
			case "subject":
				if list[i].Subject < list[j].Subject {
					return true
				} else if list[i].Subject > list[j].Subject {
					return false
				}
			case "-subject":
				if list[j].Subject < list[i].Subject {
					return true
				} else if list[j].Subject > list[i].Subject {
					return false
				}
			case "date":
				if list[i].Date.Before(list[j].Date) {
					return true
				} else if list[i].Date.After(list[j].Date) {
					return false
				}
			case "-date":
				if list[j].Date.Before(list[i].Date) {
					return true
				} else if list[j].Date.After(list[i].Date) {
					return false
				}
			case "snippet":
				if list[i].Snippet < list[j].Snippet {
					return true
				} else if list[i].Snippet > list[j].Snippet {
					return false
				}
			case "-snippet":
				if list[j].Snippet < list[i].Snippet {
					return true
				} else if list[j].Snippet > list[i].Snippet {
					return false
				}
				//default:
				//nop
			}
		}
		return false
	})
}
