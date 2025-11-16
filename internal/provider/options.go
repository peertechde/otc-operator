package provider

type Option func(o *Options)

type Options struct {
	Endpoint  string
	User      string
	Password  string
	Token     string
	AccessKey string
	SecretKey string
	Domain    string
	Project   string
	Region    string
}

func WithEndpoint(endpoint string) Option {
	return func(o *Options) {
		o.Endpoint = endpoint
	}
}

func WithUser(user string) Option {
	return func(o *Options) {
		o.User = user
	}
}

func WithPassword(password string) Option {
	return func(o *Options) {
		o.Password = password
	}
}

func WithToken(token string) Option {
	return func(o *Options) {
		o.Token = token
	}
}

func WithAccessKey(accessKey string) Option {
	return func(o *Options) {
		o.AccessKey = accessKey
	}
}

func WithSecretKey(secretKey string) Option {
	return func(o *Options) {
		o.SecretKey = secretKey
	}
}

func WithDomain(domain string) Option {
	return func(o *Options) {
		o.Domain = domain
	}
}

func WithProject(project string) Option {
	return func(o *Options) {
		o.Project = project
	}
}

func WithRegion(region string) Option {
	return func(o *Options) {
		o.Region = region
	}
}
