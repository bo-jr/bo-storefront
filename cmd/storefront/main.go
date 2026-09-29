// Command storefront answers GET /checkout?sku=X&qty=N by asking catalog for the
// item and pricing for the quote. It is the canary target of Phase 6.
// Everything except the orchestration comes from bo-service-kit.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/bo-jr/bo-service-kit/httpx"
)

// service is the in-cluster name: the metric `service` label, log field and
// OTel service.name. Never the bo- prefixed repo name.
const service = "storefront"

// maxQty matches pricing's own limit, so a request storefront accepts is one
// pricing will accept.
const maxQty = 100

// upstreamTimeout bounds each dependency call. pricing slows down under load on
// purpose; this is how long storefront is willing to wait before answering 502.
const upstreamTimeout = 10 * time.Second

type item struct {
	SKU            string `json:"sku"`
	Name           string `json:"name"`
	UnitPriceCents int64  `json:"unit_price_cents"`
}

type quoteLine struct {
	SKU            string `json:"sku"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	Qty            int    `json:"qty"`
}

type quote struct {
	TotalCents int64  `json:"total_cents"`
	Iterations int64  `json:"iterations"`
	Proof      string `json:"proof"`
}

type checkout struct {
	SKU            string `json:"sku"`
	Name           string `json:"name"`
	Qty            int    `json:"qty"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	TotalCents     int64  `json:"total_cents"`
	Proof          string `json:"proof"`
	// Version is storefront's APP_VERSION, so a canary's answers are
	// distinguishable from the stable ones by eye.
	Version string `json:"version"`
}

// errNotFound means catalog does not know the SKU: the caller's mistake, a 404.
var errNotFound = errors.New("unknown sku")

func main() {
	if err := run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	srv, err := httpx.New(ctx, service)
	if err != nil {
		return err
	}
	log := srv.Logger()
	version := srv.Chaos().AppVersion

	c := upstreams{
		client:  httpx.Client(upstreamTimeout),
		catalog: httpx.DependencyURL("catalog"),
		pricing: httpx.DependencyURL("pricing"),
	}

	srv.Handle("GET /checkout", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sku := r.URL.Query().Get("sku")
		qty, err := strconv.Atoi(r.URL.Query().Get("qty"))
		if sku == "" || err != nil || qty < 1 || qty > maxQty {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": fmt.Sprintf("want ?sku=<sku>&qty=<1..%d>", maxQty)})
			return
		}

		// Sequential on purpose: pricing needs catalog's price, and it makes
		// downstream latency add up at the edge, as Phase 6 scenario 4 needs.
		it, err := c.item(r.Context(), sku)
		if errors.Is(err, errNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown sku", "sku": sku})
			return
		}
		if err != nil {
			log.WarnContext(r.Context(), "catalog failed", "err", err.Error())
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "catalog unavailable"})
			return
		}

		q, err := c.quote(r.Context(), quoteLine{SKU: it.SKU, UnitPriceCents: it.UnitPriceCents, Qty: qty})
		if err != nil {
			log.WarnContext(r.Context(), "pricing failed", "err", err.Error())
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "pricing unavailable"})
			return
		}

		writeJSON(w, http.StatusOK, checkout{
			SKU: it.SKU, Name: it.Name, Qty: qty, UnitPriceCents: it.UnitPriceCents,
			TotalCents: q.TotalCents, Proof: q.Proof, Version: version,
		})
	}))

	return srv.Run(ctx)
}

// upstreams calls the two dependencies with a client that propagates the W3C
// traceparent of the incoming request.
type upstreams struct {
	client           *http.Client
	catalog, pricing string
}

func (u upstreams) item(ctx context.Context, sku string) (item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.catalog+"/items/"+url.PathEscape(sku), nil)
	if err != nil {
		return item{}, err
	}
	var it item
	err = u.do(req, &it)
	return it, err
}

func (u upstreams) quote(ctx context.Context, line quoteLine) (quote, error) {
	body, err := json.Marshal(map[string][]quoteLine{"lines": {line}})
	if err != nil {
		return quote{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.pricing+"/quote", bytes.NewReader(body))
	if err != nil {
		return quote{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var q quote
	err = u.do(req, &q)
	return q, err
}

func (u upstreams) do(req *http.Request, out any) error {
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		_, _ = io.Copy(io.Discard, resp.Body)
		return errNotFound
	case resp.StatusCode != http.StatusOK:
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("%s %s: %s: %s", req.Method, req.URL.Path, resp.Status, bytes.TrimSpace(msg))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
