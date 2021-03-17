package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/qdm12/ddns-updater/internal/constants"
	"github.com/qdm12/ddns-updater/internal/models"
	"github.com/qdm12/ddns-updater/internal/regex"
)

type dd24 struct {
	domain        string
	host          string
	ipVersion     models.IPVersion
	password      string
	useProviderIP bool
}

func NewDD24(data json.RawMessage, domain, host string, ipVersion models.IPVersion,
	_ regex.Matcher) (s Settings, err error) {
	extraSettings := struct {
		Password      string `json:"password"`
		UseProviderIP bool   `json:"provider_ip"`
	}{}
	if err := json.Unmarshal(data, &extraSettings); err != nil {
		return nil, err
	}
	d := &dd24{
		domain:        domain,
		host:          host,
		ipVersion:     ipVersion,
		password:      extraSettings.Password,
		useProviderIP: extraSettings.UseProviderIP,
	}
	if err := d.isValid(); err != nil {
		return nil, err
	}
	return d, nil
}

func (d *dd24) isValid() error {
	if len(d.password) == 0 {
		return ErrEmptyPassword
	}
	return nil
}

func (d *dd24) String() string {
	return toString(d.domain, d.host, constants.DD24, d.ipVersion)
}

func (d *dd24) Domain() string {
	return d.domain
}

func (d *dd24) Host() string {
	return d.host
}

func (d *dd24) IPVersion() models.IPVersion {
	return d.ipVersion
}

func (d *dd24) Proxied() bool {
	return false
}

func (d *dd24) BuildDomainName() string {
	return buildDomainName(d.host, d.domain)
}

func (d *dd24) HTML() models.HTMLRow {
	return models.HTMLRow{
		Domain:    models.HTML(fmt.Sprintf("<a href=\"http://%s\">%s</a>", d.BuildDomainName(), d.BuildDomainName())),
		Host:      models.HTML(d.Host()),
		Provider:  "<a href=\"https://www.domaindiscount24.com/\">DD24</a>",
		IPVersion: models.HTML(d.ipVersion),
	}
}

func (d *dd24) setHeaders(request *http.Request) {
	setUserAgent(request)
}

func (d *dd24) Update(ctx context.Context, client *http.Client, ip net.IP) (newIP net.IP, err error) {
	// see https://www.domaindiscount24.com/faq/en/dynamic-dns
	u := url.URL{
		Scheme: "https",
		Host:   "dynamicdns.key-systems.net",
		Path:   "/update.php",
	}
	values := url.Values{}
	values.Set("hostname", d.BuildDomainName())
	values.Set("password", d.password)
	if d.useProviderIP {
		values.Set("hostname", "auto")
	} else {
		values.Set("hostname", ip.String())
	}
	u.RawQuery = values.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(request)

	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	b, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUnmarshalResponse, err)
	}
	s := string(b)

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %d: %s",
			ErrBadHTTPStatus, response.StatusCode, bodyDataToSingleLine(s))
	}

	s = strings.ToLower(s)

	switch {
	case strings.Contains(s, "authorization failed"):
		return nil, ErrAuth
	case s == "":
		return ip, nil
	// TODO missing cases
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownResponse, s)
	}
}
