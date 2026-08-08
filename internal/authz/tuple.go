package authz

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	Wildcard = "*"
	// SystemDomain is the domain for platform-management decisions — the ones
	// that are not about any particular datasource.
	SystemDomain           = "system"
	datasourceDomainPrefix = "ds_"
	userSubjectPrefix      = "user:"
)

var ErrInvalidTuple = errors.New("invalid authorization tuple")

var allowedActions = map[string]struct{}{
	Wildcard:             {},
	"select":             {},
	"update":             {},
	"delete":             {},
	"insert":             {},
	"ddl":                {},
	"export":             {},
	"discover":           {},
	"metadata:view":      {},
	"unmask":             {},
	"desensitize:bypass": {},
	"manage":             {},
	"view":               {},
}

// DatasourceDomain returns the canonical Casbin domain for a datasource.
func DatasourceDomain(datasourceID int64) string {
	return fmt.Sprintf("%s%d", datasourceDomainPrefix, datasourceID)
}

// UserSubject returns the canonical Casbin subject for an individual user.
func UserSubject(userID int64) string {
	return fmt.Sprintf("%s%d", userSubjectPrefix, userID)
}

// NormalizeTuple validates and canonicalizes a Casbin authorization tuple.
func NormalizeTuple(sub, dom, obj, act string) (string, string, string, string, error) {
	normalizedSub, err := NormalizeSubject(sub)
	if err != nil {
		return "", "", "", "", err
	}
	normalizedDom, err := NormalizeDomain(dom)
	if err != nil {
		return "", "", "", "", err
	}
	normalizedObj := strings.TrimSpace(obj)
	if normalizedObj == "" {
		return "", "", "", "", fmt.Errorf("%w: object is required", ErrInvalidTuple)
	}
	normalizedAct := strings.ToLower(strings.TrimSpace(act))
	if _, ok := allowedActions[normalizedAct]; !ok {
		return "", "", "", "", fmt.Errorf("%w: unsupported action %q", ErrInvalidTuple, act)
	}
	return normalizedSub, normalizedDom, normalizedObj, normalizedAct, nil
}

// NormalizeSubject accepts a role slug or user:<positive-id>.
func NormalizeSubject(sub string) (string, error) {
	sub = strings.ToLower(strings.TrimSpace(sub))
	if sub == "" {
		return "", fmt.Errorf("%w: subject is required", ErrInvalidTuple)
	}
	if strings.HasPrefix(sub, userSubjectPrefix) {
		id, err := strconv.ParseInt(strings.TrimPrefix(sub, userSubjectPrefix), 10, 64)
		if err != nil || id <= 0 {
			return "", fmt.Errorf("%w: invalid user subject %q", ErrInvalidTuple, sub)
		}
		return UserSubject(id), nil
	}
	for _, ch := range sub {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '_' && ch != '-' {
			return "", fmt.Errorf("%w: invalid role subject %q", ErrInvalidTuple, sub)
		}
	}
	return sub, nil
}

// NormalizeDomain accepts "*" or a datasource domain. Bare positive IDs are
// accepted at API boundaries and converted to ds_<id>.
func NormalizeDomain(dom string) (string, error) {
	dom = strings.ToLower(strings.TrimSpace(dom))
	if dom == Wildcard {
		return Wildcard, nil
	}
	if dom == SystemDomain {
		return SystemDomain, nil
	}
	rawID := dom
	if strings.HasPrefix(dom, datasourceDomainPrefix) {
		rawID = strings.TrimPrefix(dom, datasourceDomainPrefix)
	}
	id, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || id <= 0 {
		return "", fmt.Errorf("%w: invalid datasource domain %q", ErrInvalidTuple, dom)
	}
	return DatasourceDomain(id), nil
}
