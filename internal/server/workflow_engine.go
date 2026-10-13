package server

import "sync"

type workflowEngine struct {
	server *Server
	mu     sync.Mutex
	budget []string
}

func newWorkflowEngine(server *Server) *workflowEngine { return &workflowEngine{server: server} }

func (e *workflowEngine) retryBudget(id string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, existing := range e.budget {
		if existing == id {
			return
		}
	}
	e.budget = append(e.budget, id)
}

func (e *workflowEngine) takeBudget() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids := e.budget
	e.budget = nil
	return ids
}
