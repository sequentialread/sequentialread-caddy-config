package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"regexp"
	"strings"

	envOverride "git.sequentialread.com/forest/influx-style-env-override"
	"github.com/hpcloud/tail"
)

type Config struct {
	DomainAliases                   []*DomainAlias
	IncludeDomainInKey              bool
	IncludeMethodInKey              bool
	IncludeSuccessOrFailureInKey    bool
	URIQuery                        string
	GlobalContentTypeBlacklistRegex string
	GlobalContentTypeBlacklist      *regexp.Regexp `json:"-"`
	Domains                         []*Domain
}

type Domain struct {
	MatchHostnameRegex        string
	ContentTypeWhitelistRegex string
	URIQuery                  string
	MatchHostname             *regexp.Regexp `json:"-"`
	ContentTypeWhitelist      *regexp.Regexp `json:"-"`
}

type DomainAlias struct {
	Search  string
	Replace string
}

type CaddyLog struct {
	CommonLog       string              `json:"common_log"`
	StatusCode      int                 `json:"status"`
	Request         CaddyLogRequest     `json:"request"`
	ResponseHeaders map[string][]string `json:"resp_headers"`
}

type CaddyLogRequest struct {
	URI     string              `json:"uri"`
	Host    string              `json:"host"`
	Method  string              `json:"method"`
	Headers map[string][]string `json:"headers"`
}

func main() {

	config := loadConfigFromFileAndEnvVars()

	bytez, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Fprintf(os.Stderr, "goatcounter-caddy-log-adapter using config: %s", string(bytez))

	logTailer, err := tail.TailFile(os.Args[0], tail.Config{ReOpen: true, Follow: true})
	if err != nil {
		panic(err)
	}

	for line := range logTailer.Lines {
		if line.Err != nil {
			fmt.Fprintf(os.Stderr, "unable to read next log line because %s (line: '%s')", err, line.Text)
			continue
		}

		var caddyLog CaddyLog
		err := json.Unmarshal([]byte(line.Text), &caddyLog)
		if err != nil {
			fmt.Fprintf(os.Stderr, "unable to unmarshal log line because %s (line: '%s')", err, line.Text)
			continue
		}

		referrer := ""
		userAgent := ""
		contentType := ""
		if caddyLog.Request.Headers != nil {
			referrerHeader, hasReferrerHeader := caddyLog.Request.Headers["Referrer"]
			if hasReferrerHeader && len(referrerHeader) > 0 {
				referrer = referrerHeader[0]
			}
			userAgentHeader, hasUserAgent := caddyLog.Request.Headers["User-Agent"]
			if hasUserAgent && len(userAgentHeader) > 0 {
				userAgent = userAgentHeader[0]
			}
		}
		if caddyLog.ResponseHeaders != nil {
			contentTypeHeader, hasContentTypeHeader := caddyLog.ResponseHeaders["Content-Type"]
			if hasContentTypeHeader && len(contentTypeHeader) > 0 {
				split := strings.Split(contentTypeHeader[0], ";")
				contentType = strings.TrimSpace(split[0])
			}
		}

		if config.GlobalContentTypeBlacklist != nil && config.GlobalContentTypeBlacklist.MatchString(contentType) {
			continue
		}
		var requestDomain *Domain
		for _, domain := range config.Domains {
			if domain.MatchHostname.MatchString(caddyLog.Request.Host) {
				requestDomain = domain
				break
			}
		}
		uriQuery := config.URIQuery
		if requestDomain != nil {
			if requestDomain.URIQuery != "" {
				uriQuery = requestDomain.URIQuery
			}
			if !requestDomain.ContentTypeWhitelist.MatchString(contentType) {
				continue
			}
		}

		key := caddyLog.Request.URI
		if uriQuery == "drop" {
			split := strings.Split(key, "?")
			key = split[0]
		}

		if config.IncludeMethodInKey {
			key = fmt.Sprintf("/%s%s", caddyLog.Request.Method, key)
		}

		if config.IncludeSuccessOrFailureInKey {
			isSuccess := caddyLog.StatusCode < 400
			status := "success"
			if !isSuccess {
				status = "error"
			}
			key = fmt.Sprintf("/%s%s", status, key)
		}

		if config.IncludeDomainInKey {
			host := caddyLog.Request.Host
			if config.DomainAliases != nil {
				for _, alias := range config.DomainAliases {
					host = strings.Replace(host, alias.Search, alias.Replace, 1)
				}
			}
			key = fmt.Sprintf("/%s%s", host, key)
		}

		myCommonLog := strings.Replace(caddyLog.CommonLog, caddyLog.Request.URI, key, 1)

		toPrint := fmt.Sprintf("%s:%s \"%s\" \"%s\" ", caddyLog.Request.Host, myCommonLog, referrer, userAgent)

		fmt.Fprintf(os.Stdout, toPrint)
		fmt.Fprintf(os.Stderr, toPrint)
	}

}

func loadConfigFromFileAndEnvVars() *Config {
	configBytes, err := ioutil.ReadFile("config.json")
	if err != nil {
		panic(err)
	}
	config := &Config{
		DomainAliases: []*DomainAlias{},
		Domains:       []*Domain{},
	}
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		panic(err)
	}

	// if I remember correctly I never got the silly ApplyInfluxStyleEnvOverrides thing to be able to add new entries in a list
	// so I will prepopulate the lists with 10 entries and filter the empty ones out later
	for i := 0; i < 10; i++ {
		config.DomainAliases = append(config.DomainAliases, &DomainAlias{})
	}
	for i := 0; i < 10; i++ {
		config.Domains = append(config.Domains, &Domain{})
	}

	envOverride.ApplyInfluxStyleEnvOverrides("LOGADAPTER", reflect.ValueOf(config))

	actualDomainAliases := []*DomainAlias{}
	actualDomains := []*Domain{}
	for _, alias := range config.DomainAliases {
		if alias.Search != "" {
			actualDomainAliases = append(actualDomainAliases, alias)
		}
	}
	for _, domain := range config.Domains {
		if domain.MatchHostnameRegex != "" {
			if domain.ContentTypeWhitelistRegex != "" {
				domain.ContentTypeWhitelist = regexp.MustCompile(domain.ContentTypeWhitelistRegex)
			} else {
				domain.ContentTypeWhitelist = regexp.MustCompile(".*")
			}
			domain.MatchHostname = regexp.MustCompile(domain.MatchHostnameRegex)
			actualDomains = append(actualDomains, domain)
		}
	}
	config.DomainAliases = actualDomainAliases
	config.Domains = actualDomains
	if config.GlobalContentTypeBlacklistRegex != "" {
		config.GlobalContentTypeBlacklist = regexp.MustCompile(config.GlobalContentTypeBlacklistRegex)
	}

	return config
}
