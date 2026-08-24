package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const (
	stepCreated = iota
	stepClosed
	fieldLogged
	errorLogged
	loopFieldLog
)

type requestLogContextKey struct{}

type RequestLog struct {
	mu         sync.Mutex
	started    time.Time
	logActions []logAction
}

type logAction struct {
	at     time.Time
	key    string
	value  any
	action uint8
}

type stepLog struct {
	ctx context.Context
}

type logField struct {
	Key   string
	Value any
}

func Field(key string, value any) logField {
	return logField{
		Key:   key,
		Value: value,
	}
}

func Start(ctx context.Context) context.Context {
	requestLog := &RequestLog{
		started: time.Now(),
	}

	return context.WithValue(ctx, requestLogContextKey{}, requestLog)
}

func Step(ctx context.Context, stepName string) stepLog {
	requestLog, ok := requestLogFromContext(ctx)
	if !ok {
		return stepLog{}
	}
	requestLog.mu.Lock()
	defer requestLog.mu.Unlock()
	requestLog.logActions = append(requestLog.logActions, logAction{
		at:     time.Now(),
		key:    stepName,
		action: stepCreated,
	})
	return stepLog{
		ctx: ctx,
	}
}

func (s stepLog) Close() {
	requestLog, ok := requestLogFromContext(s.ctx)
	if !ok {
		return
	}
	requestLog.mu.Lock()
	defer requestLog.mu.Unlock()
	requestLog.logActions = append(requestLog.logActions, logAction{
		at:     time.Now(),
		action: stepClosed,
	})
}

func LogFields(ctx context.Context, fields ...logField) {
	requestLog, ok := requestLogFromContext(ctx)
	if !ok {
		return
	}
	requestLog.mu.Lock()
	defer requestLog.mu.Unlock()
	for _, field := range fields {
		requestLog.logActions = append(requestLog.logActions, logAction{
			at:     time.Now(),
			key:    field.Key,
			value:  field.Value,
			action: fieldLogged,
		})
	}
}

func LogError(ctx context.Context, err error) {
	if err == nil {
		return
	}

	requestLog, ok := requestLogFromContext(ctx)
	if !ok {
		return
	}
	requestLog.mu.Lock()
	defer requestLog.mu.Unlock()
	requestLog.logActions = append(requestLog.logActions, logAction{
		at:     time.Now(),
		value:  err,
		action: errorLogged,
	})
}

func LogLoopFields(ctx context.Context, loopName string, fields ...logField) {
	requestLog, ok := requestLogFromContext(ctx)
	if !ok {
		return
	}
	requestLog.mu.Lock()
	defer requestLog.mu.Unlock()
	item := make([]logAction, 0, len(fields))
	for _, field := range fields {
		item = append(item, logAction{
			at:     time.Now(),
			key:    field.Key,
			value:  field.Value,
			action: fieldLogged,
		})
	}
	requestLog.logActions = append(requestLog.logActions, logAction{
		at:     time.Now(),
		key:    loopName,
		value:  item,
		action: loopFieldLog,
	})
}

func requestLogFromContext(ctx context.Context) (*RequestLog, bool) {
	if ctx == nil {
		return nil, false
	}
	requestLog, ok := ctx.Value(requestLogContextKey{}).(*RequestLog)
	return requestLog, ok
}

func FinishRequestLog(ctx context.Context, logger *slog.Logger, message string) {
	requestLog, ok := requestLogFromContext(ctx)
	if !ok || logger == nil {
		return
	}

	logger.InfoContext(ctx, message, requestLog.SlogArgs()...)
}

func (l *RequestLog) SlogArgs() []any {
	l.mu.Lock()
	defer l.mu.Unlock()

	root := newRequestLogScope("", l.started)
	for _, action := range l.logActions {
		root.apply(action)
	}
	ended := time.Now()
	root.closeOpenScopes(ended)
	return root.slogArgs(ended)
}

type requestLogScope struct {
	name          string
	started       time.Time
	ended         time.Time
	actions       []scopeOutputAction
	openChildren  []*requestLogScope
	parent        *requestLogScope
	root          *requestLogScope
	latestError   string
	loggingErrors []string
}

type scopeOutputAction struct {
	key   string
	value any
	kind  uint8
}

type requestLogLoopItem struct {
	actions []scopeOutputAction
	errors  []string
}

type orderedLogObject []scopeOutputAction
type orderedLogList []orderedLogObject

func newRequestLogScope(name string, started time.Time) *requestLogScope {
	scope := &requestLogScope{
		name:    name,
		started: started,
	}
	scope.root = scope
	return scope
}

func (s *requestLogScope) apply(action logAction) {
	current := s.currentOpenScope()
	switch action.action {
	case stepCreated:
		child := &requestLogScope{
			name:    action.key,
			started: action.at,
			parent:  current,
			root:    s,
		}
		current.openChildren = append(current.openChildren, child)
		current.actions = append(current.actions, scopeOutputAction{
			key:   action.key,
			value: child,
			kind:  stepCreated,
		})
	case stepClosed:
		if current == s {
			s.loggingErrors = append(s.loggingErrors, "step close was called without an open step")
			return
		}
		current.ended = action.at
		current.parent.openChildren = current.parent.openChildren[:len(current.parent.openChildren)-1]
	case fieldLogged:
		current.actions = append(current.actions, scopeOutputAction{
			key:   action.key,
			value: action.value,
			kind:  fieldLogged,
		})
	case errorLogged:
		err, ok := action.value.(error)
		if !ok {
			s.loggingErrors = append(s.loggingErrors, "logged error value was not an error")
			return
		}
		s.latestError = err.Error()
		current.actions = append(current.actions, scopeOutputAction{
			key:   "errors",
			value: err.Error(),
			kind:  errorLogged,
		})
	case loopFieldLog:
		loopActions, ok := action.value.([]logAction)
		if !ok {
			s.loggingErrors = append(s.loggingErrors, fmt.Sprintf("loop %s value was not a log action list", action.key))
			return
		}
		current.actions = append(current.actions, scopeOutputAction{
			key:   action.key,
			value: buildLoopItem(loopActions),
			kind:  loopFieldLog,
		})
	default:
		s.loggingErrors = append(s.loggingErrors, fmt.Sprintf("unknown log action %d", action.action))
	}
}

func (s *requestLogScope) currentOpenScope() *requestLogScope {
	current := s
	for len(current.openChildren) > 0 {
		current = current.openChildren[len(current.openChildren)-1]
	}
	return current
}

func (s *requestLogScope) closeOpenScopes(ended time.Time) {
	for {
		current := s.currentOpenScope()
		if current == s {
			break
		}
		s.loggingErrors = append(s.loggingErrors, fmt.Sprintf("step %s wasnt closed", current.name))
		current.ended = ended
		current.parent.openChildren = current.parent.openChildren[:len(current.parent.openChildren)-1]
	}
	s.ended = ended
}

func (s *requestLogScope) slogArgs(ended time.Time) []any {
	attrs := s.slogAttrs(ended, true)
	args := make([]any, 0, len(attrs)*2)
	for _, attr := range attrs {
		args = append(args, attr.Key, attr.Value)
	}
	return args
}

func (s *requestLogScope) slogAttrs(fallbackEnded time.Time, root bool) []slog.Attr {
	ended := s.ended
	if ended.IsZero() {
		ended = fallbackEnded
	}

	attrs := []slog.Attr{
		slog.Time("started_at", s.started),
		slog.Time("ended_at", ended),
		slog.Int64("duration_ms", ended.Sub(s.started).Milliseconds()),
	}
	attrs = append(attrs, s.outputAttrs(root)...)
	if root {
		attrs = append(attrs, slog.Any("logging_errors", s.loggingErrors))
	}
	return attrs
}

func (s *requestLogScope) outputAttrs(root bool) []slog.Attr {
	var attrs []slog.Attr
	seen := map[string]int{}
	emittedLoops := map[string]bool{}
	var errors []string

	for actionIndex, action := range s.actions {
		switch action.kind {
		case errorLogged:
			errorMessage, ok := action.value.(string)
			if !ok {
				s.root.loggingErrors = append(s.root.loggingErrors, "stored error value was not a string")
				continue
			}
			errors = append(errors, errorMessage)
		case stepCreated:
			child, ok := action.value.(*requestLogScope)
			if !ok {
				s.root.loggingErrors = append(s.root.loggingErrors, fmt.Sprintf("step %s value was not a request log scope", action.key))
				continue
			}
			attrs = append(attrs, slog.Any(nextLogKey(seen, action.key), slog.GroupValue(child.slogAttrs(s.ended, false)...)))
		case loopFieldLog:
			if emittedLoops[action.key] {
				continue
			}
			emittedLoops[action.key] = true
			item, ok := action.value.(requestLogLoopItem)
			if !ok {
				s.root.loggingErrors = append(s.root.loggingErrors, fmt.Sprintf("loop %s value was not a request log loop item", action.key))
				continue
			}
			items := orderedLogList{item.orderedObject(s.root)}
			for _, nextAction := range s.actions[actionIndex+1:] {
				if nextAction.kind != loopFieldLog || nextAction.key != action.key {
					continue
				}
				nextItem, ok := nextAction.value.(requestLogLoopItem)
				if !ok {
					s.root.loggingErrors = append(s.root.loggingErrors, fmt.Sprintf("loop %s value was not a request log loop item", nextAction.key))
					continue
				}
				items = append(items, nextItem.orderedObject(s.root))
			}
			attrs = append(attrs, slog.Any(nextLogKey(seen, action.key), items))
		default:
			attrs = append(attrs, slog.Any(nextLogKey(seen, action.key), action.value))
		}
	}

	if len(errors) > 0 {
		attrs = append(attrs, slog.Any(nextLogKey(seen, "errors"), errors))
	}
	if root && s.latestError != "" {
		attrs = append(attrs, slog.String(nextLogKey(seen, "error"), s.latestError))
	}

	return attrs
}

func buildLoopItem(actions []logAction) requestLogLoopItem {
	item := requestLogLoopItem{
		actions: make([]scopeOutputAction, 0, len(actions)),
	}
	for _, action := range actions {
		if action.action == errorLogged {
			if err, ok := action.value.(error); ok {
				item.errors = append(item.errors, err.Error())
			}
			continue
		}
		item.actions = append(item.actions, scopeOutputAction{
			key:   action.key,
			value: action.value,
			kind:  action.action,
		})
	}
	return item
}

func (i requestLogLoopItem) orderedObject(root *requestLogScope) orderedLogObject {
	var object orderedLogObject
	seen := map[string]int{}
	for _, action := range i.actions {
		if action.kind != fieldLogged {
			root.loggingErrors = append(root.loggingErrors, "loop item contained a non field action")
			continue
		}
		object = append(object, scopeOutputAction{
			key:   nextLogKey(seen, action.key),
			value: action.value,
			kind:  fieldLogged,
		})
	}
	if len(i.errors) > 0 {
		object = append(object, scopeOutputAction{
			key:   nextLogKey(seen, "errors"),
			value: i.errors,
			kind:  fieldLogged,
		})
	}
	return object
}

func nextLogKey(seen map[string]int, key string) string {
	seen[key]++
	if seen[key] == 1 {
		return key
	}
	return fmt.Sprintf("%s_%d", key, seen[key])
}

func (o orderedLogObject) MarshalJSON() ([]byte, error) {
	data := []byte{'{'}
	for i, action := range o {
		if i > 0 {
			data = append(data, ',')
		}
		key, err := json.Marshal(action.key)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(action.value)
		if err != nil {
			return nil, err
		}
		data = append(data, key...)
		data = append(data, ':')
		data = append(data, value...)
	}
	data = append(data, '}')
	return data, nil
}

func (o orderedLogObject) String() string {
	data, err := o.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("%v", []scopeOutputAction(o))
	}
	return string(data)
}

func (o orderedLogObject) asMap() map[string]any {
	out := make(map[string]any, len(o))
	for _, action := range o {
		out[action.key] = action.value
	}
	return out
}

func (l orderedLogList) MarshalJSON() ([]byte, error) {
	data := []byte{'['}
	for i, item := range l {
		if i > 0 {
			data = append(data, ',')
		}
		value, err := item.MarshalJSON()
		if err != nil {
			return nil, err
		}
		data = append(data, value...)
	}
	data = append(data, ']')
	return data, nil
}

func (l orderedLogList) String() string {
	data, err := l.MarshalJSON()
	if err != nil {
		return fmt.Sprintf("%v", []orderedLogObject(l))
	}
	return string(data)
}
