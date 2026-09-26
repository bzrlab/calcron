package server

type workflowEngine struct{ server *Server }

func newWorkflowEngine(server *Server) *workflowEngine { return &workflowEngine{server: server} }
