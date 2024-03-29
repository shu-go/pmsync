package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
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
	if err != nil {
		return xerrors.Errorf("failed to instantiate a gmail service: %v", err)
	}

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

		wg := sync.WaitGroup{}
		mut := sync.Mutex{}

		for _, msg := range resp.Messages {
			wg.Add(1)
			go func(msg *gmail.Message) {
				//m, err := msgService.Get(c.LoginID, msg.Id).Format("metadata").Do()
				m, err := msgService.Get(g.UserID, msg.Id).Format("full").Do()
				if err != nil {
					fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
					return
				}

				content := c.Format
				content = strings.ReplaceAll(content, "{id}", m.Id)
				if strings.Contains(c.Format, "{subject}") {
					content = strings.ReplaceAll(content, "{subject}", getHeader(m.Payload.Headers, "Subject"))
				}
				if strings.Contains(c.Format, "{headers}") {
					content = strings.ReplaceAll(content, "{headers}", fmt.Sprintf("%#v", m.Payload.Headers))
				}
				if strings.Contains(c.Format, "{date}") {
					content = strings.ReplaceAll(content, "{date}", getHeader(m.Payload.Headers, "Date"))
				}
				if strings.Contains(c.Format, "{snippet}") {
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
			}(msg)
		}
		wg.Wait()
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
