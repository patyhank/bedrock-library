package auth

// Go MC MIcroSoft Auth

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dlclark/regexp2"
	"golang.org/x/oauth2"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

// MSauth holds Microsoft auth credentials
type MSauth struct {
	AccessToken  string
	ExpiresAfter int64
	RefreshToken string
}

// AzureClientIDEnvVar Used to lookup Azure client id via os.Getenv if cid is not passed
const AzureClientIDEnvVar = "AzureClientID"

// RefreshMSCode Checks MSauth for expired token and refreshes if needed
func RefreshMSCode(code, cid string) (auth *MSauth, err error) {
	if cid == "" {
		cid = os.Getenv(AzureClientIDEnvVar)
	}
	MSdata := url.Values{
		"client_id": {cid},
		// "client_secret": {os.Getenv("AzureSecret")},
		"code":         {code},
		"grant_type":   {"authorization_code"},
		"redirect_uri": {"https://login.live.com/oauth20_desktop.srf"},
	}
	var MSresp *http.Response
	MSresp, err = http.PostForm("https://login.live.com/oauth20_token.srf", MSdata)
	if err != nil {
		return nil, err
	}
	var MSres map[string]interface{}
	json.NewDecoder(MSresp.Body).Decode(&MSres)
	MSresp.Body.Close()
	if MSresp.StatusCode != 200 {
		return nil, fmt.Errorf("MS refresh attempt answered not HTTP200! Instead got %s and following json: %#v", MSresp.Status, MSres)
	}
	auth = &MSauth{}
	MSaccessToken, ok := MSres["access_token"].(string)
	if !ok {
		return nil, errors.New("access_token not found in response")
	}
	auth.AccessToken = MSaccessToken
	MSrefreshToken, ok := MSres["refresh_token"].(string)
	if !ok {
		return nil, errors.New("refresh_token not found in response")
	}
	auth.RefreshToken = MSrefreshToken
	MSexpireSeconds, ok := MSres["expires_in"].(float64)
	if !ok {
		return nil, errors.New("expires_in not found in response")
	}
	auth.ExpiresAfter = time.Now().Unix() + int64(MSexpireSeconds)

	return
}

func AuthMSLogin(username, password string) (string, error) {
	client := http.DefaultClient
	ppft := regexp.MustCompile("sFTTag:[ ]?'.*value=\"(.*)\"/>'")
	urlPost := regexp2.MustCompile("urlPost:[ ]?'(.+?(?='))", 0)
	code := regexp2.MustCompile("[?|&]code=([\\w.-]+)", 0)
	loginEndpoint := fmt.Sprintf("https://login.live.com/oauth20_authorize.srf?redirect_uri=https://login.live.com/oauth20_desktop.srf&scope=service::user.auth.xboxlive.com::MBI_SSL&display=touch&response_type=code&locale=en&client_id=%v", "0000000048183522")
	req, _ := http.NewRequest("GET", loginEndpoint, nil)
	result, err := client.Do(req)
	if err != nil {
		fmt.Println(err)
		return "", err
	}
	if result.StatusCode != 200 {
		fmt.Println(err)
		return "", err
	}
	cookie := result.Header.Values("set-cookie")
	var cookies []string
	for _, s := range cookie {
		cookies = append(cookies, strings.SplitN(s, ";", 2)[0])
	}
	cookieStr := strings.Join(cookies, "; ")
	body, _ := ioutil.ReadAll(result.Body)
	mppft := ppft.FindStringSubmatch(string(body))
	mUrlPost, err := urlPost.FindStringMatch(string(body))
	if mppft == nil || err != nil || mUrlPost == nil {
		return "", errors.New("could not parse response")
	}
	//fmt.Println(mppft[1])
	mapp := map[string]string{
		"login":            username,
		"loginfmt":         username,
		"passwd":           password,
		"PPFT":             mppft[1],
		"PPSX":             "PassportR",
		"type":             "11",
		"NewUser":          "1",
		"LoginOptions":     "1",
		"i13":              "1",
		"CookieDisclosure": "0",
		"ps":               "2",
		"ctx":              "2",
		"i19":              "25774",
	}
	values := url.Values{}

	for s, s2 := range mapp {
		values.Add(s, s2)
	}
	preURL := mUrlPost.GroupByNumber(1).String()
	req, _ = http.NewRequest("POST", preURL, strings.NewReader(values.Encode()))
	//fmt.Println(preURL)

	req.Header.Add("Cookie", cookieStr)
	req.Header.Add("Pragma", "no-cache")
	req.Header.Add("Accept-Encoding", "gzip, deflate, compress")
	req.Header.Add("Accept-Language", "en-US, en;q=0.9")
	req.Header.Add("User-Agent", "XboxReplay; XboxLiveAuth/4.0")
	req.Header.Add("Accept", "*/*")
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	res, err := client.Do(req)
	//
	//all, err := io.ReadAll(res.Body)
	//
	//fmt.Println(string(all))
	//fmt.Println(all)
	//fmt.Println(res.Request.URL.String())
	currUrl := res.Request.URL.String()
	if currUrl == preURL {
		return "", nil
	}
	matchString, _ := code.FindStringMatch(currUrl)
	matched, _ := code.MatchString(currUrl)
	if !matched {
		return "", nil
	}
	return matchString.GroupByNumber(1).String(), nil
}

// AuthXBL Gets XBox Live token from Microsoft token
func AuthXBL(MStoken string) (string, error) {
	XBLdataMap := map[string]interface{}{
		"Properties": map[string]interface{}{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  MStoken,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	XBLdata, err := json.Marshal(XBLdataMap)
	if err != nil {
		return "", err
	}
	XBLreq, err := http.NewRequest(http.MethodPost, "https://user.auth.xboxlive.com/user/authenticate", bytes.NewBuffer(XBLdata))
	if err != nil {
		return "", err
	}
	XBLreq.Header.Set("Content-Type", "application/json")
	XBLreq.Header.Set("Accept", "application/json")
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: true,
		},
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}
	XBLresp, err := client.Do(XBLreq)
	if err != nil {
		return "", err
	}
	var XBLres map[string]interface{}
	json.NewDecoder(XBLresp.Body).Decode(&XBLres)
	XBLresp.Body.Close()
	if XBLresp.StatusCode != 200 {
		return "", fmt.Errorf("XBL answered not HTTP200! Instead got %s and following json: %#v", XBLresp.Status, XBLres)
	}
	XBLtoken, ok := XBLres["Token"].(string)
	if !ok {
		return "", errors.New("Token not found in XBL response")
	}
	return XBLtoken, nil
}

// XSTSauth Holds XSTS token and UHS
type XSTSauth struct {
	Token string
	UHS   string
}

// AuthXSTS Gets XSTS token using XBL
func AuthXSTS(XBLtoken string) (XSTSauth, error) {
	var auth XSTSauth
	XSTSdataMap := map[string]interface{}{
		"Properties": map[string]interface{}{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{XBLtoken},
		},
		"RelyingParty": "rp://api.minecraftservices.com/",
		"TokenType":    "JWT",
	}
	XSTSdata, err := json.Marshal(XSTSdataMap)
	if err != nil {
		return auth, err
	}
	XSTSreq, err := http.NewRequest(http.MethodPost, "https://xsts.auth.xboxlive.com/xsts/authorize", bytes.NewBuffer(XSTSdata))
	if err != nil {
		return auth, err
	}
	XSTSreq.Header.Set("Content-Type", "application/json")
	XSTSreq.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	XSTSresp, err := client.Do(XSTSreq)
	if err != nil {
		return auth, err
	}
	var XSTSres map[string]interface{}
	json.NewDecoder(XSTSresp.Body).Decode(&XSTSres)
	XSTSresp.Body.Close()
	if XSTSresp.StatusCode != 200 {
		return auth, fmt.Errorf("XSTS answered not HTTP200! Instead got %s and following json: %#v", XSTSresp.Status, XSTSres)
	}
	XSTStoken, ok := XSTSres["Token"].(string)
	if !ok {
		return auth, errors.New("Could not find Token in XSTS response")
	}
	auth.Token = XSTStoken
	XSTSdc, ok := XSTSres["DisplayClaims"].(map[string]interface{})
	if !ok {
		return auth, errors.New("Could not find DisplayClaims object in XSTS response")
	}
	XSTSxui, ok := XSTSdc["xui"].([]interface{})
	if !ok {
		return auth, errors.New("Could not find xui array in DisplayClaims object")
	}
	if len(XSTSxui) < 1 {
		return auth, errors.New("xui array in DisplayClaims object does not have any elements")
	}
	XSTSuhsObject, ok := XSTSxui[0].(map[string]interface{})
	if !ok {
		return auth, errors.New("Could not get ush object in xui array")
	}
	XSTSuhs, ok := XSTSuhsObject["uhs"].(string)
	if !ok {
		return auth, errors.New("Could not get uhs string from ush object")
	}
	auth.UHS = XSTSuhs
	return auth, nil
}

// MCauth Represents Minecraft auth response
type MCauth struct {
	Token        string
	ExpiresAfter int64
}

// AuthMC Gets Minecraft authorization from XSTS token
func AuthMC(token XSTSauth) (MCauth, error) {
	var auth MCauth
	MCdataMap := map[string]interface{}{
		"identityToken": "XBL3.0 x=" + token.UHS + ";" + token.Token,
	}
	MCdata, err := json.Marshal(MCdataMap)
	if err != nil {
		return auth, err
	}
	MCreq, err := http.NewRequest(http.MethodPost, "https://api.minecraftservices.com/authentication/login_with_xbox", bytes.NewBuffer(MCdata))
	if err != nil {
		return auth, err
	}
	MCreq.Header.Set("Content-Type", "application/json")
	MCreq.Header.Set("Accept", "application/json")
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	MCresp, err := client.Do(MCreq)
	if err != nil {
		return auth, err
	}
	var MCres map[string]interface{}
	json.NewDecoder(MCresp.Body).Decode(&MCres)
	MCresp.Body.Close()
	if MCresp.StatusCode != 200 {
		return auth, fmt.Errorf("MC answered not HTTP200! Instead got %s and following json: %#v", MCresp.Status, MCres)
	}
	MCtoken, ok := MCres["access_token"].(string)
	if !ok {
		return auth, errors.New("Could not find access_token in MC response")
	}
	auth.Token = MCtoken
	MCexpire, ok := MCres["expires_in"].(float64)
	if !ok {
		return auth, errors.New("Could not find expires_in in MC response")
	}
	auth.ExpiresAfter = time.Now().Unix() + int64(MCexpire)
	return auth, nil
}

// GetMCcredentialsByPassword From 0 to Minecraft Auth with cache using password flow
func GetMCcredentialsByPassword(username, password string) (*oauth2.Token, error) {
	var resauth *oauth2.Token
	refreshToken, err := AuthMSLogin(username, password)
	if err != nil {
		return nil, err
	}
	m, err := RefreshMSCode(refreshToken, "0000000048183522")
	if err != nil {
		return nil, err
	}
	return &oauth2.Token{
		AccessToken:  m.AccessToken,
		TokenType:    "Bearer",
		RefreshToken: m.RefreshToken,
		Expiry:       time.UnixMilli(m.ExpiresAfter),
	}, nil
	//XBLa, err := AuthXBL(m.AccessToken)
	//if err != nil {
	//	return nil, err
	//}
	//log.Println("Authorized on XBL, trying to get XSTS token...")

	//XSTSa, err := AuthXSTS(XBLa)
	//if err != nil {
	//	return nil, err
	//}
	//log.Println("Got XSTS token, trying to get MC token...")

	//MCa, err := AuthMC(XSTSa)
	//if err != nil {
	//	return nil, err
	//}
	//log.Println("Got MC token, NOT checking that you own the game because it is too complicated and going straight for MC profile...")

	//resauth, err = GetMCprofile(MCa.Token)
	//if err != nil {
	//	return resauth, err
	//}
	////log.Println("Got MC profile")
	////log.Println("UUID: " + resauth.UUID)
	////log.Println("Name: " + resauth.Name)
	//resauth.AsTk = MCa.Token
	return resauth, nil
}
