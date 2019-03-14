package cmd

import (
	"os"
    "runtime"

	log "github.com/sirupsen/logrus"

	"github.com/spf13/cobra"

    "slurp/scanner/stats"
)

// CLI args
var cfgDebug bool
var cfgConcurrency int
var cfgPermutationsFile string
var cfgAWSRegion string
var cfgKeywords []string
var cfgDomains []string

var state string

// cobra vars
var rootCmd = &cobra.Command{}
var domainCmd = &cobra.Command{}
var keywordCmd = &cobra.Command{}
var internalCmd = &cobra.Command{}

type Config struct {
	Debug            bool
	Concurrency      int
    Region           string
	PermutationsFile string
    State            string
    Keywords         []string
    Domains          []string
    Stats            *stats.Stats
}

func setFlags() {
    domainCmd.PersistentFlags().StringSliceVarP(&cfgDomains, "target", "t", []string{}, "Domains to enumerate s3 buckets; format: example1.com,example2.com,example3.com")
	domainCmd.PersistentFlags().StringVarP(&cfgPermutationsFile, "permutations", "p", "./permutations.json", "Permutations file location")
	domainCmd.PersistentFlags().BoolVarP(&cfgDebug, "debug", "d", false, "Debug output")
	domainCmd.PersistentFlags().IntVarP(&cfgConcurrency, "concurrency", "c", 0, "Connection concurrency; default is the system CPU count")

	keywordCmd.PersistentFlags().StringSliceVarP(&cfgKeywords, "target", "t", []string{}, "List of keywords to enumerate s3; format: keyword1,keyword2,keyword3")
	keywordCmd.PersistentFlags().StringVarP(&cfgPermutationsFile, "permutations", "p", "./permutations.json", "Permutations file location")
	keywordCmd.PersistentFlags().BoolVarP(&cfgDebug, "debug", "d", false, "Debug output")
	keywordCmd.PersistentFlags().IntVarP(&cfgConcurrency, "concurrency", "c", 0, "Connection concurrency; default is the system CPU count")

    internalCmd.PersistentFlags().StringVarP(&cfgAWSRegion, "region", "r", "us-west-2", "AWS Region to connect to")
}

func NewCmd(useDesc, shortDesc, longDesc, st string) *cobra.Command {
	return &cobra.Command{
		Use:   useDesc,
		Short: shortDesc,
		Long:  longDesc,
		Run: func(cmd *cobra.Command, args []string) {
			state = st
		},
	}
}

// CmdInit initializes goroutine concurrency and initializes cobra
func CmdInit(useDesc, shortDesc, longDesc string) Config {
	rootCmd = NewCmd(useDesc, shortDesc, longDesc, "ROOT")
    domainCmd = NewCmd("domain", "Domain based scanning mode", "Domain based scanning mode", "DOMAIN")
	keywordCmd = NewCmd("keyword", "Keyword based scanning mode", "Domain based scanning mode", "KEYWORD")
    internalCmd = NewCmd("internal", "Scan based on AWS credentials", "Scan based on AWS credentials", "INTERNAL")

    setFlags()

	helpCmd := rootCmd.HelpFunc()

	var helpFlag bool

	newHelpCmd := func(c *cobra.Command, args []string) {
		helpFlag = true
		helpCmd(c, args)
	}
	rootCmd.SetHelpFunc(newHelpCmd)

	// domainCmd command help
	helpDomainCmd := domainCmd.HelpFunc()
	newDomainHelpCmd := func(c *cobra.Command, args []string) {
		helpFlag = true
		helpDomainCmd(c, args)
	}
	domainCmd.SetHelpFunc(newDomainHelpCmd)

	// keywordCmd command help
	helpKeywordCmd := keywordCmd.HelpFunc()
	newKeywordHelpCmd := func(c *cobra.Command, args []string) {
		helpFlag = true
		helpKeywordCmd(c, args)
	}
	keywordCmd.SetHelpFunc(newKeywordHelpCmd)

    // internalCmd command help
	helpInternalCmd := internalCmd.HelpFunc()
	newInternalHelpCmd := func(c *cobra.Command, args []string) {
		helpFlag = true
		helpInternalCmd(c, args)
	}
	internalCmd.SetHelpFunc(newInternalHelpCmd)

	// Add subcommands
	rootCmd.AddCommand(domainCmd)
	rootCmd.AddCommand(keywordCmd)
    rootCmd.AddCommand(internalCmd)

	err := rootCmd.Execute()

	if err != nil {
		log.Fatal(err)
	}

	if cfgDebug {
		log.SetLevel(log.DebugLevel)
	}

	if cfgConcurrency == 0 || cfgConcurrency < 0 {
		cfgConcurrency = runtime.NumCPU()
	}

	if helpFlag {
		os.Exit(0)
	}

    return Config{
		Debug:            cfgDebug,
		Concurrency:      cfgConcurrency,
        PermutationsFile: cfgPermutationsFile,
        Region:           cfgAWSRegion,
        State:            state,
        Keywords:         cfgKeywords,
        Domains:          cfgDomains,
        Stats:            stats.NewStats(),
	}
}
