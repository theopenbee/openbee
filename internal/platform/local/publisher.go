package local

import "sync"

const (
	taskStateQueued    = "queued"
	taskStateRunning   = "running"
	taskStateCompleted = "completed"
	taskStateFailed    = "failed"
	taskStateCancelled = "cancelled"
)

type taskActivity struct {
	state       string
	queuedAhead int
}

type EventPublisher struct {
	hub   *SSEHub
	mu    sync.Mutex
	tasks map[string]map[string]taskActivity
}

func NewEventPublisher(hub *SSEHub) *EventPublisher {
	return &EventPublisher{hub: hub, tasks: make(map[string]map[string]taskActivity)}
}

func (p *EventPublisher) Message(sessionKey string, msg ChatMessage) {
	userID, conversationID, ok := ParseSessionKey(sessionKey)
	if !ok {
		return
	}
	p.hub.Publish(userID, Event{Type: EventMessage, ConversationID: conversationID, Message: &msg})
}

func (p *EventPublisher) TaskState(sessionKey, taskID, workerID, state string, queuedAhead int) {
	userID, conversationID, ok := ParseSessionKey(sessionKey)
	if !ok {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.record(sessionKey, taskID, state, queuedAhead)
	p.hub.Publish(userID, Event{
		Type:           EventTask,
		ConversationID: conversationID,
		TaskID:         taskID,
		WorkerID:       workerID,
		State:          state,
		QueuedAhead:    queuedAhead,
	})
}

func (p *EventPublisher) WorkerStatus(workerID, status string) {
	p.hub.PublishAll(Event{Type: EventWorkerStatus, WorkerID: workerID, Status: status})
}

func (p *EventPublisher) Activity(sessionKey string) Activity {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := Activity{State: ActivityIdle}
	for _, t := range p.tasks[sessionKey] {
		switch t.state {
		case taskStateRunning:
			return Activity{State: ActivityRunning}
		case taskStateQueued:
			if result.State == ActivityIdle || t.queuedAhead < result.QueuedAhead {
				result = Activity{State: ActivityQueued, QueuedAhead: t.queuedAhead}
			}
		}
	}
	return result
}

func (p *EventPublisher) record(sessionKey, taskID, state string, queuedAhead int) {
	switch state {
	case taskStateCompleted, taskStateFailed, taskStateCancelled:
		delete(p.tasks[sessionKey], taskID)
		if len(p.tasks[sessionKey]) == 0 {
			delete(p.tasks, sessionKey)
		}
		return
	}
	if p.tasks[sessionKey] == nil {
		p.tasks[sessionKey] = make(map[string]taskActivity)
	}
	p.tasks[sessionKey][taskID] = taskActivity{state: state, queuedAhead: queuedAhead}
}
