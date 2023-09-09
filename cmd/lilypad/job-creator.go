package lilypad

import (
	"github.com/bacalhau-project/lilypad/pkg/jobcreator"
	"github.com/bacalhau-project/lilypad/pkg/system"
	"github.com/bacalhau-project/lilypad/pkg/web3"
	"github.com/spf13/cobra"
)

func NewJobCreatorOptions() jobcreator.JobCreatorOptions {
	return jobcreator.JobCreatorOptions{
		Web3: getDefaultWeb3Options(),
	}
}

func newJobCreatorCmd() *cobra.Command {
	options := NewJobCreatorOptions()

	solverCmd := &cobra.Command{
		Use:     "job-creator",
		Short:   "Start the lilypad resource-provider service.",
		Long:    "Start the lilypad resource-provider service.",
		Example: "",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runJobCreator(cmd, options)
		},
	}

	addWeb3CliFlags(solverCmd, options.Web3)

	return solverCmd
}

func runJobCreator(cmd *cobra.Command, options jobcreator.JobCreatorOptions) error {
	commandCtx := system.NewCommandContext(cmd)
	defer commandCtx.Cleanup()

	web3SDK, err := web3.NewContractSDK(options.Web3)
	if err != nil {
		return err
	}

	solver, err := jobcreator.NewJobCreator(options, web3SDK)
	if err != nil {
		return err
	}

	err = solver.Start(commandCtx.Ctx, commandCtx.Cm)
	if err != nil {
		return err
	}

	<-commandCtx.Ctx.Done()
	return nil
}
