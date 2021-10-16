package main

import (
	"golang.org/x/xerrors"
)

type authCmd struct {
	_ struct{} `help:""`

	Port int `default:"7676"  help:"a temporal port for OAuth authentication. 0 is for copy&paste to CLI."`
}

func (c authCmd) Run(g globalCmd) error {
	config, err := getConfig(g.Credentials, g.ClientID, g.ClientSecret)
	if err != nil {
		return xerrors.Errorf("failed to get config: %v", err)
	}

	/*client*/
	tok, err := getTokenFromWeb(config, uint16(c.Port))
	if err != nil {
		return err
	}

	err = saveToken(g.Token, tok)
	if err != nil {
		return err
	}

	return nil
}
