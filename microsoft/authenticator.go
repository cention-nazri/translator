package microsoft

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const scope = "http://api.microsofttranslator.com"

type Authenticator interface {
	Authenticate(request *http.Request) error
}

type authenticator struct {
	provider        AuthenticationProvider
	accessTokenChan chan *accessToken
}

func newAuthenticator(clientId, clientSecret string) Authenticator {
	// make buffered accessToken channel an pre-fill it with an expired token
	tokenChan := make(chan *accessToken, 1)
	tokenChan <- &accessToken{}

	// return new authenticator that uses the above accessToken channel
	return &authenticator{
		provider:        newAuthenticationProvider(clientId, clientSecret),
		accessTokenChan: tokenChan,
	}
}

func (a *authenticator) Authenticate(request *http.Request) error {
	authToken, err := a.authToken()
	if err != nil {
		return err
	}

	request.Header.Add("Authorization", authToken)
	return nil
}

func (a *authenticator) authToken() (string, error) {
	// grab the token
	accessToken := <-a.accessTokenChan

	// make sure it's valid, otherwise request a new one
	if accessToken == nil || accessToken.expired() {
		err := a.provider.RefreshAccessToken(accessToken)
		if err != nil || accessToken == nil {
			a.accessTokenChan <- nil
			return "", err
		}
	}

	// put the token back on the channel
	a.accessTokenChan <- accessToken

	// return authToken
	return "Bearer " + accessToken.Token, nil
}

type accessToken struct {
	Token     string `json:"access_token"`
	Type      string `json:"token_type"`
	Scope     string `json:"scope"`
	ExpiresIn string `json:"expires_in"`
	ExpiresAt time.Time
}

func (t *accessToken) expired() bool {
	// be conservative and expire 10 seconds early
	return t.ExpiresAt.Before(time.Now().Add(time.Second * 10))
}

type AuthenticationProvider interface {
	RefreshAccessToken(*accessToken) error
}

type authenticationProvider struct {
	clientId     string
	clientSecret string
	router       Router
}

func newAuthenticationProvider(clientId, clientSecret string) AuthenticationProvider {
	return &authenticationProvider{
		clientId:     clientId,
		clientSecret: clientSecret,
		router:       newRouter(),
	}
}

func (p *authenticationProvider) RefreshAccessToken(token *accessToken) error {
	values := make(url.Values)
	values.Set("client_id", p.clientId)
	values.Set("client_secret", p.clientSecret)
	values.Set("scope", scope)
	values.Set("grant_type", "client_credentials")

	response, err := http.PostForm(p.router.AuthUrl(), values)
	if err != nil {
		log.Println(err)
		return err
	}

	body, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		log.Println(err)
		return err
	}

	if err := json.Unmarshal(body, token); err != nil {
		log.Println(err)
		return err
	}

	expiresInSeconds, err := strconv.Atoi(token.ExpiresIn)
	if err != nil {
		log.Println(err)
		return err
	}

	token.ExpiresAt = time.Now().Add(time.Duration(expiresInSeconds) * time.Second)

	return nil
}
