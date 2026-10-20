package terminal

type readerDocumentObserverToken struct {
	active bool
}

type readerDocumentObservers struct {
	subscriptions map[*readerDocumentObserverToken]func(readerDocument)
}

func (o *readerDocumentObservers) observe(observer func(readerDocument), initial readerDocument) func() {
	if observer == nil {
		return func() {}
	}
	if o.subscriptions == nil {
		o.subscriptions = make(map[*readerDocumentObserverToken]func(readerDocument))
	}
	token := &readerDocumentObserverToken{active: true}
	o.subscriptions[token] = observer
	observer(initial)
	return func() {
		if !token.active {
			return
		}
		token.active = false
		delete(o.subscriptions, token)
	}
}

func (o *readerDocumentObservers) notify(document readerDocument) {
	for token, observer := range o.subscriptions {
		if token.active {
			observer(document)
		}
	}
}
