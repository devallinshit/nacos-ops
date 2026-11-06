package config

type ConfigID struct {
	DataID    string
	Group     string
	Namespace string
}

type Config struct {
	ID      ConfigID
	Content string
}
