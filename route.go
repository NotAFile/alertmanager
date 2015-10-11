package main

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/alertmanager/config"
	"github.com/prometheus/alertmanager/types"
)

var DefaultRouteOpts = RouteOpts{
	GroupWait:      20 * time.Second,
	GroupInterval:  5 * time.Minute,
	RepeatInterval: 1 * time.Hour,
	SendResolved:   true,
}

type Routes []*Route

func (rs Routes) Match(lset model.LabelSet) []*RouteOpts {
	fakeParent := &Route{
		Routes:    rs,
		RouteOpts: DefaultRouteOpts,
	}
	return fakeParent.Match(lset)
}

// A Route is a node that contains definitions of how to handle alerts.
type Route struct {
	// The configuration parameters for matches of this route.
	RouteOpts RouteOpts

	// Equality or regex matchers an alert has to fulfill to match
	// this route.
	Matchers types.Matchers

	// If true, an alert matches further routes on the same level.
	Continue bool

	// Children routes of this route.
	Routes Routes
}

func NewRoute(cr *config.Route, parent *RouteOpts) *Route {
	groupBy := map[model.LabelName]struct{}{}
	for _, ln := range cr.GroupBy {
		groupBy[ln] = struct{}{}
	}

	// Create default and overwrite with configured settings.
	opts := *parent
	opts.GroupBy = groupBy

	if cr.SendTo != "" {
		opts.SendTo = cr.SendTo
	}
	if cr.GroupWait != nil {
		opts.GroupWait = time.Duration(*cr.GroupWait)
	}
	if cr.GroupInterval != nil {
		opts.GroupInterval = time.Duration(*cr.GroupInterval)
	}
	if cr.RepeatInterval != nil {
		opts.RepeatInterval = time.Duration(*cr.RepeatInterval)
	}
	if cr.SendResolved != nil {
		opts.SendResolved = *cr.SendResolved
	}

	// Build matchers.
	var matchers types.Matchers

	for ln, lv := range cr.Match {
		matchers = append(matchers, types.NewMatcher(model.LabelName(ln), lv))
	}
	for ln, lv := range cr.MatchRE {
		m, err := types.NewRegexMatcher(model.LabelName(ln), lv.String())
		if err != nil {
			// Must have been sanitized during config validation.
			panic(err)
		}
		matchers = append(matchers, m)
	}

	return &Route{
		RouteOpts: opts,
		Matchers:  matchers,
		Continue:  cr.Continue,
		Routes:    NewRoutes(cr.Routes, &opts),
	}
}

func NewRoutes(croutes []*config.Route, parent *RouteOpts) Routes {
	if parent == nil {
		parent = &DefaultRouteOpts
	}
	res := Routes{}
	for _, cr := range croutes {
		res = append(res, NewRoute(cr, parent))
	}
	return res
}

// Match does a depth-first left-to-right search through the route tree
// and returns the flattened configuration for the reached node.
func (r *Route) Match(lset model.LabelSet) []*RouteOpts {
	if !r.Matchers.Match(lset) {
		return nil
	}

	var all []*RouteOpts

	for _, cr := range r.Routes {
		matches := cr.Match(lset)

		all = append(all, matches...)

		if matches != nil && !cr.Continue {
			break
		}
	}

	if len(all) == 0 {
		all = append(all, &r.RouteOpts)
	}

	return all
}

type RouteOpts struct {
	// The identifier of the associated notification configuration
	SendTo       string
	SendResolved bool

	// What labels to group alerts by for notifications.
	GroupBy map[model.LabelName]struct{}

	// How long to wait to group matching alerts before sending
	// a notificaiton
	GroupWait      time.Duration
	GroupInterval  time.Duration
	RepeatInterval time.Duration
}

func (ro *RouteOpts) String() string {
	var labels []model.LabelName
	for ln := range ro.GroupBy {
		labels = append(labels, ln)
	}
	return fmt.Sprintf("<RouteOpts send_to:%q group_by:%q timers:%q|%q>", ro.SendTo, labels, ro.GroupWait, ro.GroupInterval)
}