package terminal

type workbenchConcern uint8

const (
	workbenchCancellationOwnership workbenchConcern = iota
	workbenchResumeOutbox
	workbenchRunOutbox
	workbenchSteerOutbox
	workbenchDraft
	workbenchHistory
	workbenchConcernCount
	workbenchSessionConcernCount = workbenchHistory
)

type workbenchHealth struct {
	problems [workbenchConcernCount]string
}

func (w *workbenchHealth) update(concern workbenchConcern, err error) bool {
	problem := ""
	if err != nil {
		problem = "workbench: " + err.Error()
	}
	if w.problems[concern] == problem {
		return false
	}
	w.problems[concern] = problem
	return true
}

func (w *workbenchHealth) problem() string {
	for _, problem := range w.problems {
		if problem != "" {
			return problem
		}
	}
	return ""
}

func (w *workbenchHealth) enterSession() {
	for concern := range workbenchSessionConcernCount {
		w.problems[concern] = ""
	}
}

func (a *app) reportWorkbenchIssue(concern workbenchConcern, err error) {
	if a.workbenchHealth.update(concern, err) {
		a.status.setProblem(a.workbenchHealth.problem())
	}
}
