// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.yaml.in/yaml/v4"
)

type schema struct {
	tag    string
	fields map[string]*schema
	item   *schema
	any    bool
}

func scalar(tag string) *schema                { return &schema{tag: tag} }
func object(fields map[string]*schema) *schema { return &schema{tag: "!!map", fields: fields} }
func array(item *schema) *schema               { return &schema{tag: "!!seq", item: item} }
func dict(item *schema) *schema                { return &schema{tag: "!!map", item: item} }

var stringSchema = scalar("!!str")
var boolSchema = scalar("!!bool")
var intSchema = scalar("!!int")
var stringsSchema = array(stringSchema)
var routerSchema = object(map[string]*schema{"rule": stringSchema, "ruleSyntax": stringSchema, "service": stringSchema, "entryPoints": stringsSchema, "middlewares": stringsSchema, "priority": intSchema, "tls": object(map[string]*schema{"certResolver": stringSchema, "options": stringSchema, "domains": array(object(map[string]*schema{"main": stringSchema, "sans": stringsSchema}))})})
var serviceSchema = object(map[string]*schema{"loadBalancer": object(map[string]*schema{"servers": array(object(map[string]*schema{"url": stringSchema, "weight": intSchema, "preservePath": boolSchema})), "passHostHeader": boolSchema, "serversTransport": stringSchema, "responseForwarding": object(map[string]*schema{"flushInterval": stringSchema}), "healthCheck": object(map[string]*schema{"path": stringSchema, "scheme": stringSchema, "mode": stringSchema, "method": stringSchema, "status": intSchema, "port": intSchema, "interval": stringSchema, "timeout": stringSchema, "hostname": stringSchema, "followRedirects": boolSchema, "headers": dict(stringSchema)}), "sticky": object(map[string]*schema{"cookie": object(map[string]*schema{"name": stringSchema, "secure": boolSchema, "httpOnly": boolSchema, "sameSite": stringSchema, "maxAge": intSchema})})}), "weighted": object(map[string]*schema{"services": array(object(map[string]*schema{"name": stringSchema, "weight": intSchema})), "sticky": &schema{tag: "!!map", any: true}, "healthCheck": object(nil)}), "mirroring": object(map[string]*schema{"service": stringSchema, "maxBodySize": intSchema, "mirrors": array(object(map[string]*schema{"name": stringSchema, "percent": intSchema}))}), "failover": object(map[string]*schema{"service": stringSchema, "fallback": stringSchema, "healthCheck": object(nil)})})
var middlewareSchema = object(map[string]*schema{
	"redirectScheme": object(map[string]*schema{"scheme": stringSchema, "port": stringSchema, "permanent": boolSchema}), "redirectRegex": object(map[string]*schema{"regex": stringSchema, "replacement": stringSchema, "permanent": boolSchema}),
	"headers":     object(map[string]*schema{"frameDeny": boolSchema, "browserXssFilter": boolSchema, "contentTypeNosniff": boolSchema, "forceSTSHeader": boolSchema, "stsSeconds": intSchema, "stsIncludeSubdomains": boolSchema, "stsPreload": boolSchema, "customFrameOptionsValue": stringSchema, "contentSecurityPolicy": stringSchema, "referrerPolicy": stringSchema, "permissionsPolicy": stringSchema, "customRequestHeaders": dict(stringSchema), "customResponseHeaders": dict(stringSchema), "accessControlAllowCredentials": boolSchema, "accessControlAllowHeaders": stringsSchema, "accessControlAllowMethods": stringsSchema, "accessControlAllowOriginList": stringsSchema, "accessControlAllowOriginListRegex": stringsSchema, "accessControlExposeHeaders": stringsSchema, "accessControlMaxAge": intSchema, "addVaryHeader": boolSchema}),
	"forwardAuth": object(map[string]*schema{"address": stringSchema, "trustForwardHeader": boolSchema, "authRequestHeaders": stringsSchema, "authResponseHeaders": stringsSchema, "authResponseHeadersRegex": stringSchema, "forwardBody": boolSchema, "maxBodySize": intSchema, "preserveLocationHeader": boolSchema, "preserveRequestMethod": boolSchema, "addAuthCookiesToResponse": stringsSchema}),
	"plugin":      dict(&schema{tag: "!!map", any: true}), "compress": object(map[string]*schema{"excludedContentTypes": stringsSchema, "includedContentTypes": stringsSchema, "minResponseBodyBytes": intSchema, "encodings": stringsSchema, "defaultEncoding": stringSchema}), "stripPrefix": object(map[string]*schema{"prefixes": stringsSchema, "forceSlash": boolSchema}), "stripPrefixRegex": object(map[string]*schema{"regex": stringsSchema}), "addPrefix": object(map[string]*schema{"prefix": stringSchema}), "replacePath": object(map[string]*schema{"path": stringSchema}), "replacePathRegex": object(map[string]*schema{"regex": stringSchema, "replacement": stringSchema}), "chain": object(map[string]*schema{"middlewares": stringsSchema}), "basicAuth": object(map[string]*schema{"users": stringsSchema, "usersFile": stringSchema, "realm": stringSchema, "removeHeader": boolSchema, "headerField": stringSchema}), "ipAllowList": object(map[string]*schema{"sourceRange": stringsSchema, "rejectStatusCode": intSchema}), "retry": object(map[string]*schema{"attempts": intSchema, "initialInterval": stringSchema}),
})
var graphSchema = object(map[string]*schema{"http": object(map[string]*schema{"routers": dict(routerSchema), "services": dict(serviceSchema), "middlewares": dict(middlewareSchema), "serversTransports": dict(object(map[string]*schema{"serverName": stringSchema, "insecureSkipVerify": boolSchema, "rootCAs": stringsSchema, "maxIdleConnsPerHost": intSchema}))}), "tls": object(map[string]*schema{"certificates": array(object(map[string]*schema{"certFile": stringSchema, "keyFile": stringSchema, "stores": stringsSchema})), "options": dict(object(map[string]*schema{"minVersion": stringSchema, "maxVersion": stringSchema, "cipherSuites": stringsSchema, "curvePreferences": stringsSchema, "sniStrict": boolSchema, "alpnProtocols": stringsSchema, "clientAuth": object(map[string]*schema{"caFiles": stringsSchema, "clientAuthType": stringSchema})})), "stores": dict(object(map[string]*schema{"defaultCertificate": object(map[string]*schema{"certFile": stringSchema, "keyFile": stringSchema}), "defaultGeneratedCert": object(map[string]*schema{"resolver": stringSchema, "domain": object(map[string]*schema{"main": stringSchema, "sans": stringsSchema})})}))})})

func validateShape(n *yaml.Node, s *schema, p string) error {
	if n.Tag != s.tag {
		return problem(dto.InvalidGraph, p, n.Line, "wrong field type")
	}
	if s.any {
		return nil
	}
	switch n.Kind {
	case yaml.MappingNode:
		for j := 0; j < len(n.Content); j += 2 {
			k, v := n.Content[j], n.Content[j+1]
			child := s.item
			if child == nil {
				child = s.fields[k.Value]
			}
			if child == nil {
				return problem(dto.UnsupportedFormat, p, k.Line, "field outside validated subset: "+k.Value)
			}
			if err := validateShape(v, child, p); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, v := range n.Content {
			if err := validateShape(v, s.item, p); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateDocument(d *document, p string) error {
	if err := validateShape(d.root, graphSchema, p); err != nil {
		return err
	}
	routers := at(d.root, "http", "routers")
	for _, key := range keys(routers) {
		r := field(routers, key)
		syntax := str(field(r, "ruleSyntax"))
		if syntax != "" && syntax != "v2" && syntax != "v3" {
			return problem(dto.UnsupportedFormat, p, r.Line, "unknown router rule syntax")
		}
		rule := field(r, "rule")
		if rule == nil {
			return problem(dto.InvalidGraph, p, r.Line, "router rule required")
		}
		if str(field(r, "ruleSyntax")) != "v2" {
			if _, err := parseRule(str(rule)); err != nil {
				return problem(dto.InvalidGraph, p, rule.Line, err.Error())
			}
		}
		if str(field(r, "service")) == "" {
			return problem(dto.InvalidGraph, p, r.Line, "router service required")
		}
	}
	services := at(d.root, "http", "services")
	for _, key := range keys(services) {
		s := field(services, key)
		if len(keys(s)) != 1 {
			return problem(dto.InvalidGraph, p, s.Line, "exactly one service type required")
		}
		servers := at(s, "loadBalancer", "servers")
		if servers != nil {
			for _, server := range servers.Content {
				if !validURL(str(field(server, "url"))) {
					return problem(dto.InvalidGraph, p, server.Line, "invalid server URL")
				}
			}
		}
	}
	for _, key := range keys(at(d.root, "http", "middlewares")) {
		m := at(d.root, "http", "middlewares", key)
		if len(keys(m)) != 1 {
			return problem(dto.InvalidGraph, p, m.Line, "exactly one middleware type required")
		}
	}
	return nil
}
func validURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.User == nil && u.Fragment == "" && !strings.ContainsAny(s, "\x00\r\n")
}
func (e *engine) Validate(ctx context.Context, in proxy.Intent) error {
	g, err := e.graph(ctx, in.Request.Instance)
	if err != nil {
		return err
	}
	files, _, err := e.run.Copy(ctx, in.Request.Instance, path.Join(in.Request.Instance.Source.MountTarget, in.TempPath), true)
	if err != nil {
		return err
	}
	if len(files) != 1 || proxy.Snapshot([]byte(files[0].Content), true).FileState != in.New.FileState {
		return dto.Fail(dto.InvalidGraph, 422)
	}
	// Validate the staged candidate in the complete currently consumed graph.
	docs := map[string]*document{}
	for _, f := range g.result.Files {
		if f.Path == in.Request.Target {
			continue
		}
		d, err := parseDocument(f.Content, f.Path)
		if err != nil {
			return err
		}
		if err = validateDocument(d, f.Path); err != nil {
			return err
		}
		docs[f.Path] = d
	}
	d, err := parseDocument(files[0].Content, in.Request.Target)
	if err != nil {
		return err
	}
	if err = validateDocument(d, in.Request.Target); err != nil {
		return err
	}
	docs[in.Request.Target] = d
	if g.readErr != nil {
		return g.readErr
	}
	if !g.result.Inventory.Eligibility.Writable {
		return eligibilityError(g.result.Inventory.Eligibility)
	}
	if err = validateDefinitions(docs); err != nil {
		return err
	}
	return checkNewReferences(docs, g.result.OpaqueFiles, g.result.Inventory, in)
}
func eligibilityError(v dto.Eligibility) error { return problem(v.Reason, v.Path, v.Line, v.Detail) }

func validateDefinitions(docs map[string]*document) error {
	seen := map[string]string{}
	for p, d := range docs {
		for _, section := range []string{"routers", "services", "middlewares", "serversTransports"} {
			for _, key := range keys(at(d.root, "http", section)) {
				id := section + "#" + key
				if prev := seen[id]; prev != "" {
					return problem(dto.InvalidGraph, p, at(d.root, "http", section, key).Line, "duplicate definition also in "+prev)
				}
				seen[id] = p
			}
		}
	}
	return nil
}
