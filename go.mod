module slurp

require (
	github.com/Workiva/go-datastructures v1.0.44
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/jmoiron/jsonq v0.0.0-20150511023944-e874b168d07e
	github.com/joeguo/tldextract v0.0.0-20180214020933-b623e0574407
	github.com/onsi/gomega v1.4.2 // indirect
	github.com/sirupsen/logrus v1.4.0
	github.com/spf13/cobra v0.0.1
	github.com/spf13/pflag v1.0.0 // indirect
	golang.org/x/net v0.0.0-20180906233101-161cd47e91fd
	gopkg.in/airbrake/gobrake.v2 v2.0.9 // indirect
	gopkg.in/gemnasium/logrus-airbrake-hook.v2 v2.1.2 // indirect
	scanner/external v0.0.0
)

replace scanner/external => ./scanner/external
