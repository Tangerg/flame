package delivery

const FeedbackCreate Name = "feedback.create"

func registerFeedback(registry *Registry) {
	registry.CommandAck(MethodMeta{Name: FeedbackCreate},
		(*Handler).CreateFeedback)
}
