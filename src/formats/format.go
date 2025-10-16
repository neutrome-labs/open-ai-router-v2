package formats

type Format interface {
	FromJson(data []byte) error
	ToJson() ([]byte, error)
}

type ReqFormat interface {
	Format
	FromChatCompletions(req Format) error
}

type ResFormat interface {
	Format
	ToChatCompletions(req Format) error
}
