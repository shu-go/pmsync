package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/pkg/browser"
	"github.com/shu-go/gli"
	"golang.org/x/oauth2"
	google "golang.org/x/oauth2/google"
	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
)

// Version is app version
var Version string

func init() {
	if Version == "" {
		Version = "dev-" + time.Now().Format("20060102")
	}
}

type globalCmd struct {
	UserID string `cli:"userid" default:"me"`
	Label  string `cli:"label,box" default:"Notes/pomera_sync"`

	Credentials string `cli:"credentials,c=FILE_NAME"  default:"./credentials.json"  help:"your client configuration file from Google Developer Console"`
	Token       string `cli:"token,t=FILE_NAME"  default:"./token.json"  help:"file path to read/write retrieved token"`

	ClientID, ClientSecret string `help:"if no credentials.json"`
	AuthPort               uint16 `cli:"auth-port=NUMBER"  default:"7878"`

	Auth authCmd `help:"update token"`
	List listCmd `cli:"list,ls" help:"list notes(mail messages)" usage:"args accepts Gmail advanced search syntax (https://support.google.com/mail/answer/7190)"`
	Get  getCmd  `help:"display or download as a file"`
	Put  putCmd  `help:"upload files as notes(gmail messages)"`
}

//var scopes = []string{gmail.MailGoogleComScope}
var scopes = []string{gmail.GmailLabelsScope, gmail.GmailModifyScope}

func getConfig(credentialsFilePath string, clientID, clientSecret string) (*oauth2.Config, error) {
	var config *oauth2.Config
	if _, err := os.Stat(credentialsFilePath); err != nil {
		if clientID == "" || clientSecret == "" {
			return nil, xerrors.New("ClientID or ClientSecret is empty")
		}

		config = &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  google.Endpoint.AuthURL,  //"https://accounts.google.com/o/oauth2/auth",
				TokenURL: google.Endpoint.TokenURL, //"https://accounts.google.com/o/oauth2/token",
			},
		}
	} else {
		b, err := ioutil.ReadFile(credentialsFilePath)
		if err != nil {
			return nil, xerrors.Errorf("reading credentials file: %v", err)
		}

		// If modifying these scopes, delete your previously saved token.json.
		/*
			config, err = google.ConfigFromJSON(b, gmail.MailGoogleComScope)
			if err != nil {
				return nil, xerrors.Errorf("credentials: %v", err)
			}
		*/
		// borrowed from https://cs.opensource.google/go/x/oauth2/+/6b3c2da3:google/google.go;l=38
		type cred struct {
			ClientID     string   `json:"client_id"`
			ClientSecret string   `json:"client_secret"`
			RedirectURIs []string `json:"redirect_uris"`
			AuthURI      string   `json:"auth_uri"`
			TokenURI     string   `json:"token_uri"`
		}
		var j struct {
			Web       *cred `json:"web"`
			Installed *cred `json:"installed"`
		}
		if err := json.Unmarshal(b, &j); err != nil {
			return nil, fmt.Errorf("unmarshal: %v", err)
		}
		var c *cred
		switch {
		case j.Web != nil:
			c = j.Web
		case j.Installed != nil:
			c = j.Installed
		default:
			return nil, fmt.Errorf("no credentials found")
		}
		config = &oauth2.Config{
			ClientID:     c.ClientID,
			ClientSecret: c.ClientSecret,
			RedirectURL:  c.RedirectURIs[0],
			Scopes:       scopes,
			Endpoint: oauth2.Endpoint{
				AuthURL:  c.AuthURI,
				TokenURL: c.TokenURI,
			},
		}
	}

	return config, nil

}

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config, tokFile string, port uint16) (*http.Client, *oauth2.Token, error) {
	// The file token.json stores the user's access and refresh tokens, and is
	// created automatically when the authorization flow completes for the first
	// time.
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok, err = getTokenFromWeb(config, port)
		if err != nil {
			return nil, nil, err
		}

		err := saveToken(tokFile, tok)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}
	return config.Client(context.Background(), tok), tok, nil
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config, port uint16) (*oauth2.Token, error) {
	// setup parameters

	var codeChan chan string
	config.RedirectURL = fmt.Sprintf("http://localhost:%d/", port)
	codeChan = make(chan string)
	go launchRedirectionServer(port, codeChan)

	// request authorization (and authentication)

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	browser.OpenURL(authURL)

	var authCode string
	authCode = <-codeChan

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		return nil, xerrors.Errorf("failed to retrieve token from web: %v", err)
	}
	return tok, nil
}

func launchRedirectionServer(port uint16, codeChan chan string) {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")
		codeChan <- code

		var color string
		var icon string
		var result string
		if code != "" {
			//success
			color = "green"
			icon = "&#10003;"
			result = "Successfully authenticated!!"
		} else {
			//fail
			color = "red"
			icon = "&#10008;"
			result = "FAILED!"
		}
		disp := fmt.Sprintf(`<div><span style="font-size:xx-large; color:%s; border:solid thin %s;">%s</span> %s</div>`, color, color, icon, result)

		fmt.Fprintf(w, `
<html>
	<head><title>%s pomi</title></head>
	<body onload="open(location, '_self').close();"> <!-- Chrome won't let me close! -->
		%s
		<hr />
		<p>This is a temporal page.<br />Please close it.</p>
	</body>
</html>
`, icon, disp)
	})
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) error {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		xerrors.Errorf("failed to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)

	return nil
}

func getHeader(headers []*gmail.MessagePartHeader, key string) string {
	for _, h := range headers {
		if h.Name == key {
			return h.Value
		}
	}
	return ""
}

func main() {
	app := gli.NewWith(&globalCmd{})
	app.Name = "pmsync"
	app.Desc = "Gmail<-->file sync for Pomera DM200"
	app.Version = Version
	app.Usage = `* create credentials at https://console.developers.google.com/apis/credentials
* download credentials.json
* pmsync auth
* pmsync get
* pmsync get -o file`
	app.Copyright = "(C) 2021 Shuhei Kubota"
	app.Run(os.Args)

}
