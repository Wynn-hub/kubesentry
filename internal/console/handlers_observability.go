package console

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/Wynn-hub/kubesentry/internal/admissionobs"
	"github.com/Wynn-hub/kubesentry/internal/api/v1alpha1"
	"github.com/Wynn-hub/kubesentry/internal/dbrepo"
	"github.com/Wynn-hub/kubesentry/internal/gateway"
)

const (
	observationHealthWindow = 5 * time.Minute
	observationFreshFor     = 90 * time.Second
)

type observationFreshness struct {
	LastObservedAt *time.Time `json:"lastObservedAt"`
	Fresh          bool       `json:"fresh"`
}

type observationFaults struct {
	Dropped        int64 `json:"dropped"`
	RetryExhausted int64 `json:"retryExhausted"`
	BatchFailure   int64 `json:"batchFailure"`
	AuthFailure    int64 `json:"authFailure"`
	ACKTimeout     int64 `json:"ackTimeout"`
	ReportDropped  int64 `json:"reportDropped"`
}

func (f observationFaults) total() int64 {
	return f.Dropped + f.BatchFailure + f.AuthFailure + f.ACKTimeout + f.ReportDropped
}

func (f *observationFaults) add(aggregate dbrepo.ObservationAggregate) {
	f.Dropped += aggregate.Dropped
	f.RetryExhausted += aggregate.RetryExhausted
	f.BatchFailure += aggregate.BatchFailure
	f.AuthFailure += aggregate.AuthFailure
	f.ACKTimeout += aggregate.ACKTimeout
	f.ReportDropped += aggregate.ReportDropped
}

type observationQueue struct {
	DepthMax int64 `json:"depthMax"`
	Capacity int64 `json:"capacity"`
}

type observationReason struct {
	Component string `json:"component"`
	Reason    string `json:"reason"`
	Count     int64  `json:"count"`
}

type observationClusterCounts struct {
	Total         int `json:"total"`
	Online        int `json:"online"`
	Normal        int `json:"normal"`
	Warning       int `json:"warning"`
	PartialNoData int `json:"partialNoData"`
	NoData        int `json:"noData"`
	NotOnboarded  int `json:"notOnboarded"`
	Offline       int `json:"offline"`
}

type observationPlatformHealth struct {
	Health                 string `json:"health"`
	CleanupFailure         int64  `json:"cleanupFailure"`
	ObservationWriteFailed int64  `json:"observationWriteFailed"`
	AuthFailure            int64  `json:"authFailure"`
}

type observationOverviewResponse struct {
	Health                   string                          `json:"health"`
	DataIncomplete           bool                            `json:"dataIncomplete"`
	BaselineReset            bool                            `json:"baselineReset"`
	ObservabilityOnboarded   bool                            `json:"observabilityOnboarded"`
	PlatformHealth           observationPlatformHealth       `json:"platformHealth"`
	Freshness                map[string]observationFreshness `json:"freshness"`
	Generated                int64                           `json:"generated"`
	OperatorReceived         int64                           `json:"operatorReceived"`
	GatewayReceived          int64                           `json:"gatewayReceived"`
	Written                  int64                           `json:"written"`
	PolicyFilteredMember     int64                           `json:"policyFilteredMember"`
	PolicyFilteredGateway    int64                           `json:"policyFilteredGateway"`
	RetentionElapsed         int64                           `json:"retentionElapsed"`
	Duplicate                int64                           `json:"duplicate"`
	QueueRejected            map[string]int64                `json:"queueRejected"`
	RetryCount               map[string]int64                `json:"retryCount"`
	WindowFaults             map[string]observationFaults    `json:"windowFaults"`
	WindowFaultTotal         int64                           `json:"windowFaultTotal"`
	SeriesStarted            map[string]int64                `json:"seriesStarted"`
	SequenceGap              map[string]int64                `json:"sequenceGap"`
	BaselineResetByComponent map[string]int64                `json:"baselineResetByComponent"`
	Queues                   map[string]observationQueue     `json:"queues"`
	FaultReasons             []observationReason             `json:"faultReasons"`
	ClusterCounts            observationClusterCounts        `json:"clusterCounts"`
	From                     time.Time                       `json:"from"`
	To                       time.Time                       `json:"to"`
}

type clusterObservationState struct {
	health     string
	freshness  map[string]observationFreshness
	queues     map[string]observationQueue
	faults     observationFaults
	reasons    []observationReason
	incomplete bool
}

func (h *Handlers) admissionObservationOverview(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Observability == nil || h.Repos.Clusters == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	from, to, now, ok := h.observationWindow(w, r)
	if !ok {
		return
	}
	cluster, clusterID, ok := h.observationCluster(w, r)
	if !ok {
		return
	}
	windowAggregates, err := h.observationAggregates(r, clusterID, from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	healthAggregates, err := h.observationAggregates(r, clusterID, now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	platformID := int64(0)
	platformAggregates, err := h.Repos.Observability.Aggregate(r.Context(), &platformID, now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	clusters, err := h.Repos.Clusters.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	fixedByCluster, err := h.Repos.Observability.AggregateClusters(r.Context(), now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sustained, err := h.Repos.Observability.SustainedPressure(r.Context(), now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	states := observationStates(clusters, groupClusterAggregates(fixedByCluster), sustained, now)
	counts := countObservationClusterStates(clusters, states)

	window := aggregateComponents(windowAggregates)
	health := aggregateComponents(healthAggregates)
	response := observationOverviewResponse{
		Health:                   overviewHealth(counts),
		ObservabilityOnboarded:   cluster != nil && cluster.ObservabilityOnboardedAt != nil,
		PlatformHealth:           platformObservationHealth(platformAggregates),
		Freshness:                componentFreshness(health, now),
		QueueRejected:            map[string]int64{},
		RetryCount:               map[string]int64{},
		WindowFaults:             map[string]observationFaults{},
		SeriesStarted:            map[string]int64{},
		SequenceGap:              map[string]int64{},
		BaselineResetByComponent: map[string]int64{},
		Queues:                   map[string]observationQueue{},
		ClusterCounts:            counts,
		From:                     from,
		To:                       to,
	}
	if cluster != nil {
		response.Health = states[cluster.ID].health
		response.Freshness = states[cluster.ID].freshness
	}
	if cluster == nil {
		response.ObservabilityOnboarded = counts.Online > 0 && counts.NotOnboarded == 0
	}
	for component, aggregate := range window {
		response.QueueRejected[component] = aggregate.QueueRejected
		response.RetryCount[component] = aggregate.Retry
		response.SeriesStarted[component] = aggregate.SeriesStartedCount
		response.SequenceGap[component] = aggregate.SequenceGapCount
		response.BaselineResetByComponent[component] = aggregate.BaselineResetCount
		response.Queues[component] = observationQueue{DepthMax: aggregate.QueueDepthMax, Capacity: aggregate.QueueCapacity}
		faults := observationFaults{}
		faults.add(aggregate)
		response.WindowFaults[component] = faults
		response.WindowFaultTotal += faults.total()
		if aggregate.SequenceGapCount > 0 {
			response.DataIncomplete = true
		}
		if aggregate.BaselineResetCount > 0 {
			response.BaselineReset = true
		}
		for reason, count := range aggregate.FaultReasonCounts {
			response.FaultReasons = append(response.FaultReasons, observationReason{Component: component, Reason: reason, Count: count})
		}
	}
	sort.Slice(response.FaultReasons, func(i, j int) bool {
		if response.FaultReasons[i].Count == response.FaultReasons[j].Count {
			return response.FaultReasons[i].Reason < response.FaultReasons[j].Reason
		}
		return response.FaultReasons[i].Count > response.FaultReasons[j].Count
	})
	response.Generated = componentCounter(window, admissionobs.ComponentWebhook, func(a dbrepo.ObservationAggregate) int64 { return a.Generated })
	response.OperatorReceived = componentCounter(window, admissionobs.ComponentOperator, func(a dbrepo.ObservationAggregate) int64 { return a.Received })
	response.GatewayReceived = componentCounter(window, admissionobs.ComponentGateway, func(a dbrepo.ObservationAggregate) int64 { return a.Received })
	response.Written = componentCounter(window, admissionobs.ComponentGateway, func(a dbrepo.ObservationAggregate) int64 { return a.Written })
	response.PolicyFilteredMember = componentCounter(window, admissionobs.ComponentOperator, func(a dbrepo.ObservationAggregate) int64 { return a.PolicyFiltered })
	response.PolicyFilteredGateway = componentCounter(window, admissionobs.ComponentGateway, func(a dbrepo.ObservationAggregate) int64 { return a.PolicyFiltered })
	response.RetentionElapsed = componentCounter(window, admissionobs.ComponentGateway, func(a dbrepo.ObservationAggregate) int64 { return a.RetentionElapsed })
	response.Duplicate = componentCounter(window, admissionobs.ComponentGateway, func(a dbrepo.ObservationAggregate) int64 { return a.Duplicate })
	writeOK(w, response)
}

func (h *Handlers) observationAggregates(r *http.Request, clusterID *int64, from, to time.Time) ([]dbrepo.ObservationAggregate, error) {
	if clusterID != nil {
		return h.Repos.Observability.Aggregate(r.Context(), clusterID, from, to)
	}
	byCluster, err := h.Repos.Observability.AggregateClusters(r.Context(), from, to)
	if err != nil {
		return nil, err
	}
	return mergeClusterAggregates(byCluster), nil
}

func (h *Handlers) observationWindow(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, time.Time, bool) {
	settings, err := h.Repos.Observability.GetSettings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	now := time.Now().UTC()
	to := now
	from := now.Add(-24 * time.Hour)
	if value := r.URL.Query().Get("to"); value != "" {
		to, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "to must be RFC3339")
			return time.Time{}, time.Time{}, time.Time{}, false
		}
	}
	if value := r.URL.Query().Get("from"); value != "" {
		from, err = time.Parse(time.RFC3339, value)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "from must be RFC3339")
			return time.Time{}, time.Time{}, time.Time{}, false
		}
	}
	if to.After(now) {
		to = now
	}
	cutoff := now.Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour)
	if from.Before(cutoff) {
		from = cutoff
	}
	from, to = from.UTC(), to.UTC()
	if !from.Before(to) {
		writeErr(w, http.StatusBadRequest, "from must be before to within the retention period")
		return time.Time{}, time.Time{}, time.Time{}, false
	}
	return from, to, now, true
}

func (h *Handlers) observationCluster(w http.ResponseWriter, r *http.Request) (*dbrepo.Cluster, *int64, bool) {
	name := strings.TrimSpace(r.URL.Query().Get("cluster"))
	if name == "" {
		return nil, nil, true
	}
	cluster, err := h.Repos.Clusters.GetByName(r.Context(), name)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeErr(w, http.StatusNotFound, "cluster not found")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return nil, nil, false
	}
	return cluster, &cluster.ID, true
}

func aggregateComponents(aggregates []dbrepo.ObservationAggregate) map[string]dbrepo.ObservationAggregate {
	result := make(map[string]dbrepo.ObservationAggregate)
	for _, aggregate := range aggregates {
		current := result[aggregate.Component]
		current.Component = aggregate.Component
		current.Counters = current.Counters.Add(aggregate.Counters)
		current.ObservationWriteFailed += aggregate.ObservationWriteFailed
		current.SequenceGapCount += aggregate.SequenceGapCount
		current.BaselineResetCount += aggregate.BaselineResetCount
		current.SeriesStartedCount += aggregate.SeriesStartedCount
		if aggregate.QueueDepthMax > current.QueueDepthMax {
			current.QueueDepthMax = aggregate.QueueDepthMax
		}
		if aggregate.QueueCapacity > current.QueueCapacity {
			current.QueueCapacity = aggregate.QueueCapacity
		}
		if aggregate.LastObservedAt != nil && (current.LastObservedAt == nil || aggregate.LastObservedAt.After(*current.LastObservedAt)) {
			value := *aggregate.LastObservedAt
			current.LastObservedAt = &value
		}
		current.FaultReasonCounts = addReasonCounts(current.FaultReasonCounts, aggregate.FaultReasonCounts)
		current.PressureReasonCounts = addReasonCounts(current.PressureReasonCounts, aggregate.PressureReasonCounts)
		current.FilterReasonCounts = addReasonCounts(current.FilterReasonCounts, aggregate.FilterReasonCounts)
		result[aggregate.Component] = current
	}
	return result
}

func addReasonCounts(current, incoming map[string]int64) map[string]int64 {
	if current == nil {
		current = map[string]int64{}
	}
	for reason, count := range incoming {
		current[reason] += count
	}
	return current
}

func mergeClusterAggregates(aggregates []dbrepo.ObservationAggregate) []dbrepo.ObservationAggregate {
	grouped := aggregateComponents(aggregates)
	result := make([]dbrepo.ObservationAggregate, 0, len(grouped))
	for _, aggregate := range grouped {
		result = append(result, aggregate)
	}
	return result
}

func groupClusterAggregates(aggregates []dbrepo.ObservationAggregate) map[int64][]dbrepo.ObservationAggregate {
	result := make(map[int64][]dbrepo.ObservationAggregate)
	for _, aggregate := range aggregates {
		result[aggregate.ClusterID] = append(result[aggregate.ClusterID], aggregate)
	}
	return result
}

func componentCounter(components map[string]dbrepo.ObservationAggregate, component admissionobs.Component, get func(dbrepo.ObservationAggregate) int64) int64 {
	return get(components[string(component)])
}

func componentFreshness(components map[string]dbrepo.ObservationAggregate, now time.Time) map[string]observationFreshness {
	result := make(map[string]observationFreshness, 2)
	for _, component := range []admissionobs.Component{admissionobs.ComponentWebhook, admissionobs.ComponentOperator} {
		aggregate := components[string(component)]
		fresh := aggregate.LastObservedAt != nil && now.Sub(*aggregate.LastObservedAt) <= observationFreshFor
		result[string(component)] = observationFreshness{LastObservedAt: aggregate.LastObservedAt, Fresh: fresh}
	}
	return result
}

func observationStates(
	clusters []dbrepo.Cluster,
	aggregates map[int64][]dbrepo.ObservationAggregate,
	sustained map[int64]bool,
	now time.Time,
) map[int64]clusterObservationState {
	result := make(map[int64]clusterObservationState, len(clusters))
	for _, cluster := range clusters {
		components := aggregateComponents(aggregates[cluster.ID])
		freshness := componentFreshness(components, now)
		state := clusterObservationState{
			freshness: freshness,
			queues:    map[string]observationQueue{},
		}
		for component, aggregate := range components {
			state.queues[component] = observationQueue{DepthMax: aggregate.QueueDepthMax, Capacity: aggregate.QueueCapacity}
			state.faults.add(aggregate)
			state.incomplete = state.incomplete || aggregate.SequenceGapCount > 0
			for reason, count := range aggregate.FaultReasonCounts {
				state.reasons = append(state.reasons, observationReason{Component: component, Reason: reason, Count: count})
			}
		}
		switch {
		case cluster.Phase != v1alpha1.ClusterPhaseOnline:
			state.health = "offline"
		case cluster.ObservabilityOnboardedAt == nil:
			state.health = "not_onboarded"
		case !freshness[string(admissionobs.ComponentWebhook)].Fresh && !freshness[string(admissionobs.ComponentOperator)].Fresh:
			state.health = "no_data"
		case freshness[string(admissionobs.ComponentWebhook)].Fresh != freshness[string(admissionobs.ComponentOperator)].Fresh:
			state.health = "partial_no_data"
		case state.faults.total() > 0 || sustained[cluster.ID]:
			state.health = "warning"
		default:
			state.health = "normal"
		}
		sort.Slice(state.reasons, func(i, j int) bool { return state.reasons[i].Count > state.reasons[j].Count })
		result[cluster.ID] = state
	}
	return result
}

func countObservationClusterStates(clusters []dbrepo.Cluster, states map[int64]clusterObservationState) observationClusterCounts {
	counts := observationClusterCounts{Total: len(clusters)}
	for _, cluster := range clusters {
		state := states[cluster.ID]
		switch state.health {
		case "normal":
			counts.Online++
			counts.Normal++
		case "warning":
			counts.Online++
			counts.Warning++
		case "partial_no_data":
			counts.Online++
			counts.PartialNoData++
		case "no_data":
			counts.Online++
			counts.NoData++
		case "not_onboarded":
			counts.Online++
			counts.NotOnboarded++
		default:
			counts.Offline++
		}
	}
	return counts
}

func overviewHealth(counts observationClusterCounts) string {
	switch {
	case counts.Warning > 0:
		return "warning"
	case counts.PartialNoData > 0:
		return "partial_no_data"
	case counts.NoData > 0:
		return "no_data"
	case counts.NotOnboarded > 0:
		return "not_onboarded"
	case counts.Normal > 0:
		return "normal"
	default:
		return "no_data"
	}
}

func platformObservationHealth(aggregates []dbrepo.ObservationAggregate) observationPlatformHealth {
	result := observationPlatformHealth{Health: "normal"}
	for _, aggregate := range aggregates {
		result.CleanupFailure += aggregate.CleanupFailure
		result.ObservationWriteFailed += aggregate.ObservationWriteFailed
		result.AuthFailure += aggregate.AuthFailure
	}
	if result.CleanupFailure > 0 || result.ObservationWriteFailed > 0 || result.AuthFailure > 0 {
		result.Health = "warning"
	}
	return result
}

type observationTrendPoint struct {
	BucketStartedAt  time.Time `json:"bucketStartedAt"`
	Present          bool      `json:"present"`
	Generated        int64     `json:"generated"`
	OperatorReceived int64     `json:"operatorReceived"`
	GatewayReceived  int64     `json:"gatewayReceived"`
	Written          int64     `json:"written"`
	PolicyFiltered   int64     `json:"policyFiltered"`
	Faults           int64     `json:"faults"`
}

func (h *Handlers) admissionObservationTrend(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Observability == nil || h.Repos.Clusters == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	from, to, _, ok := h.observationWindow(w, r)
	if !ok {
		return
	}
	_, clusterID, ok := h.observationCluster(w, r)
	if !ok {
		return
	}
	step := observationTrendStep(to.Sub(from))
	rows, err := h.Repos.Observability.Trend(r.Context(), clusterID, from, to, step)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	start := time.Unix((from.Unix()/int64(step.Seconds()))*int64(step.Seconds()), 0).UTC()
	points := make(map[time.Time]*observationTrendPoint)
	for bucket := start; bucket.Before(to); bucket = bucket.Add(step) {
		points[bucket] = &observationTrendPoint{BucketStartedAt: bucket}
	}
	for _, row := range rows {
		point := points[row.BucketStartedAt.UTC()]
		if point == nil {
			continue
		}
		point.Present = true
		switch admissionobs.Component(row.Component) {
		case admissionobs.ComponentWebhook:
			point.Generated += row.Generated
		case admissionobs.ComponentOperator:
			point.OperatorReceived += row.Received
			point.PolicyFiltered += row.PolicyFiltered
		case admissionobs.ComponentGateway:
			point.GatewayReceived += row.Received
			point.Written += row.Written
			point.PolicyFiltered += row.PolicyFiltered
		}
		point.Faults += observationFaults{
			Dropped: row.Dropped, BatchFailure: row.BatchFailure, AuthFailure: row.AuthFailure,
			ACKTimeout: row.ACKTimeout, ReportDropped: row.ReportDropped,
		}.total()
	}
	result := make([]observationTrendPoint, 0, len(points))
	for bucket := start; bucket.Before(to); bucket = bucket.Add(step) {
		result = append(result, *points[bucket])
	}
	writeOK(w, map[string]any{"stepSeconds": int64(step.Seconds()), "items": result, "from": from, "to": to})
}

func observationTrendStep(span time.Duration) time.Duration {
	switch {
	case span <= time.Hour:
		return time.Minute
	case span <= 6*time.Hour:
		return 2 * time.Minute
	case span <= 24*time.Hour:
		return 5 * time.Minute
	case span <= 3*24*time.Hour:
		return 15 * time.Minute
	case span <= 7*24*time.Hour:
		return time.Hour
	case span <= 30*24*time.Hour:
		return 6 * time.Hour
	case span <= 90*24*time.Hour:
		return 24 * time.Hour
	default:
		return 3 * 24 * time.Hour
	}
}

type observationClusterItem struct {
	Name                   string                          `json:"name"`
	Phase                  string                          `json:"phase"`
	Health                 string                          `json:"health"`
	ObservabilityOnboarded bool                            `json:"observabilityOnboarded"`
	Freshness              map[string]observationFreshness `json:"freshness"`
	Queues                 map[string]observationQueue     `json:"queues"`
	WindowFaults           observationFaults               `json:"windowFaults"`
	MajorFaultReasons      []observationReason             `json:"majorFaultReasons"`
	DataIncomplete         bool                            `json:"dataIncomplete"`
}

func (h *Handlers) admissionObservationClusters(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Observability == nil || h.Repos.Clusters == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	from, to, now, ok := h.observationWindow(w, r)
	if !ok {
		return
	}
	page, pageSize, ok := observationPage(w, r)
	if !ok {
		return
	}
	clusters, err := h.Repos.Clusters.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	windowAggregates, err := h.Repos.Observability.AggregateClusters(r.Context(), from, to)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	fixedAggregates, err := h.Repos.Observability.AggregateClusters(r.Context(), now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	sustained, err := h.Repos.Observability.SustainedPressure(r.Context(), now.Add(-observationHealthWindow), now)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	states := observationStates(clusters, groupClusterAggregates(fixedAggregates), sustained, now)
	window := groupClusterAggregates(windowAggregates)
	allowedHealth := observationHealthFilter(r.URL.Query().Get("health"))
	items := make([]observationClusterItem, 0, len(clusters))
	for _, cluster := range clusters {
		state := states[cluster.ID]
		if len(allowedHealth) > 0 && !allowedHealth[state.health] {
			continue
		}
		item := observationClusterItem{
			Name: cluster.Name, Phase: cluster.Phase, Health: state.health,
			ObservabilityOnboarded: cluster.ObservabilityOnboardedAt != nil,
			Freshness:              state.freshness, Queues: state.queues, DataIncomplete: state.incomplete,
		}
		for _, aggregate := range window[cluster.ID] {
			item.WindowFaults.add(aggregate)
			for reason, count := range aggregate.FaultReasonCounts {
				item.MajorFaultReasons = append(item.MajorFaultReasons,
					observationReason{Component: aggregate.Component, Reason: reason, Count: count})
			}
		}
		sort.Slice(item.MajorFaultReasons, func(i, j int) bool { return item.MajorFaultReasons[i].Count > item.MajorFaultReasons[j].Count })
		if len(item.MajorFaultReasons) > 3 {
			item.MajorFaultReasons = item.MajorFaultReasons[:3]
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	total := len(items)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	writeOK(w, map[string]any{"items": items[start:end], "total": total, "page": page, "pageSize": pageSize})
}

func observationPage(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	page, pageSize := 1, 20
	var err error
	if value := r.URL.Query().Get("page"); value != "" {
		page, err = strconv.Atoi(value)
	}
	if err == nil {
		if value := r.URL.Query().Get("pageSize"); value != "" {
			pageSize, err = strconv.Atoi(value)
		}
	}
	if err != nil || page < 1 || pageSize < 1 || pageSize > 100 {
		writeErr(w, http.StatusBadRequest, "page must be positive and pageSize must be between 1 and 100")
		return 0, 0, false
	}
	return page, pageSize, true
}

func observationHealthFilter(value string) map[string]bool {
	result := map[string]bool{}
	for _, health := range strings.Split(value, ",") {
		health = strings.TrimSpace(health)
		if health != "" {
			result[health] = true
		}
	}
	return result
}

type observationSettingsResponse struct {
	RetentionDays int `json:"retentionDays"`
}

func (h *Handlers) getObservationSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Observability == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	settings, err := h.Repos.Observability.GetSettings(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(w, observationSettingsResponse{RetentionDays: settings.RetentionDays})
}

func (h *Handlers) updateObservationSettings(w http.ResponseWriter, r *http.Request) {
	if h.Repos.Observability == nil {
		writeErr(w, http.StatusServiceUnavailable, "platform observability is unavailable")
		return
	}
	var request observationSettingsResponse
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if request.RetentionDays < 1 || request.RetentionDays > 365 {
		writeErr(w, http.StatusBadRequest, "retentionDays must be between 1 and 365")
		return
	}
	if err := h.Repos.Observability.UpdateSettings(r.Context(), request.RetentionDays); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.notifyChange(r, gateway.PlatformObservabilitySettingsTopic)
	h.getObservationSettings(w, r)
}
