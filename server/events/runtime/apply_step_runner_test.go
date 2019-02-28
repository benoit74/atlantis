package runtime_test

import (
	"fmt"
	"github.com/hashicorp/go-version"
	. "github.com/petergtz/pegomock"
	mocks2 "github.com/runatlantis/atlantis/server/events/mocks"
	"github.com/runatlantis/atlantis/server/events/mocks/matchers"
	"github.com/runatlantis/atlantis/server/events/models"
	"github.com/runatlantis/atlantis/server/events/runtime"
	"github.com/runatlantis/atlantis/server/events/terraform"
	"github.com/runatlantis/atlantis/server/events/terraform/mocks"
	matchers2 "github.com/runatlantis/atlantis/server/events/terraform/mocks/matchers"
	"github.com/runatlantis/atlantis/server/events/yaml/valid"
	"github.com/runatlantis/atlantis/server/logging"
	. "github.com/runatlantis/atlantis/testing"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestRun_NoDir(t *testing.T) {
	o := runtime.ApplyStepRunner{
		TerraformExecutor: nil,
	}
	_, err := o.Run(models.ProjectCommandContext{
		RepoRelDir: ".",
		Workspace:  "workspace",
	}, nil, "/nonexistent/path")
	ErrEquals(t, "no plan found at path \".\" and workspace \"workspace\"–did you run plan?", err)
}

func TestRun_NoPlanFile(t *testing.T) {
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	o := runtime.ApplyStepRunner{
		TerraformExecutor: nil,
	}
	_, err := o.Run(models.ProjectCommandContext{
		RepoRelDir: ".",
		Workspace:  "workspace",
	}, nil, tmpDir)
	ErrEquals(t, "no plan found at path \".\" and workspace \"workspace\"–did you run plan?", err)
}

func TestRun_Success(t *testing.T) {
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	planPath := filepath.Join(tmpDir, "workspace.tfplan")
	err := ioutil.WriteFile(planPath, nil, 0644)
	Ok(t, err)

	RegisterMockTestingT(t)
	terraform := mocks.NewMockClient()
	o := runtime.ApplyStepRunner{
		TerraformExecutor: terraform,
	}

	When(terraform.RunCommandWithVersion(matchers.AnyPtrToLoggingSimpleLogger(), AnyString(), AnyStringSlice(), matchers2.AnyPtrToGoVersionVersion(), AnyString())).
		ThenReturn("output", nil)
	output, err := o.Run(models.ProjectCommandContext{
		Workspace:   "workspace",
		RepoRelDir:  ".",
		CommentArgs: []string{"comment", "args"},
	}, []string{"extra", "args"}, tmpDir)
	Ok(t, err)
	Equals(t, "output", output)
	terraform.VerifyWasCalledOnce().RunCommandWithVersion(nil, tmpDir, []string{"apply", "-input=false", "-no-color", "extra", "args", "comment", "args", fmt.Sprintf("%q", planPath)}, nil, "workspace")
	_, err = os.Stat(planPath)
	Assert(t, os.IsNotExist(err), "planfile should be deleted")
}

func TestRun_AppliesCorrectProjectPlan(t *testing.T) {
	// When running for a project, the planfile has a different name.
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	planPath := filepath.Join(tmpDir, "projectname-default.tfplan")
	err := ioutil.WriteFile(planPath, nil, 0644)
	Ok(t, err)

	RegisterMockTestingT(t)
	terraform := mocks.NewMockClient()
	o := runtime.ApplyStepRunner{
		TerraformExecutor: terraform,
	}

	When(terraform.RunCommandWithVersion(matchers.AnyPtrToLoggingSimpleLogger(), AnyString(), AnyStringSlice(), matchers2.AnyPtrToGoVersionVersion(), AnyString())).
		ThenReturn("output", nil)
	projectName := "projectname"
	output, err := o.Run(models.ProjectCommandContext{
		Workspace:  "default",
		RepoRelDir: ".",
		ProjectConfig: &valid.Project{
			Name: &projectName,
		},
		CommentArgs: []string{"comment", "args"},
	}, []string{"extra", "args"}, tmpDir)
	Ok(t, err)
	Equals(t, "output", output)
	terraform.VerifyWasCalledOnce().RunCommandWithVersion(nil, tmpDir, []string{"apply", "-input=false", "-no-color", "extra", "args", "comment", "args", fmt.Sprintf("%q", planPath)}, nil, "default")
	_, err = os.Stat(planPath)
	Assert(t, os.IsNotExist(err), "planfile should be deleted")
}

func TestRun_UsesConfiguredTFVersion(t *testing.T) {
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	planPath := filepath.Join(tmpDir, "workspace.tfplan")
	err := ioutil.WriteFile(planPath, nil, 0644)
	Ok(t, err)

	RegisterMockTestingT(t)
	terraform := mocks.NewMockClient()
	o := runtime.ApplyStepRunner{
		TerraformExecutor: terraform,
	}
	tfVersion, _ := version.NewVersion("0.11.0")

	When(terraform.RunCommandWithVersion(matchers.AnyPtrToLoggingSimpleLogger(), AnyString(), AnyStringSlice(), matchers2.AnyPtrToGoVersionVersion(), AnyString())).
		ThenReturn("output", nil)
	output, err := o.Run(models.ProjectCommandContext{
		Workspace:   "workspace",
		RepoRelDir:  ".",
		CommentArgs: []string{"comment", "args"},
		ProjectConfig: &valid.Project{
			TerraformVersion: tfVersion,
		},
	}, []string{"extra", "args"}, tmpDir)
	Ok(t, err)
	Equals(t, "output", output)
	terraform.VerifyWasCalledOnce().RunCommandWithVersion(nil, tmpDir, []string{"apply", "-input=false", "-no-color", "extra", "args", "comment", "args", fmt.Sprintf("%q", planPath)}, tfVersion, "workspace")
	_, err = os.Stat(planPath)
	Assert(t, os.IsNotExist(err), "planfile should be deleted")
}

// Apply ignores the -target flag when used with a planfile so we should give
// an error if it's being used with -target.
func TestRun_UsingTarget(t *testing.T) {
	cases := []struct {
		commentFlags []string
		extraArgs    []string
		expErr       bool
	}{
		{
			commentFlags: []string{"-target", "mytarget"},
			expErr:       true,
		},
		{
			commentFlags: []string{"-target=mytarget"},
			expErr:       true,
		},
		{
			extraArgs: []string{"-target", "mytarget"},
			expErr:    true,
		},
		{
			extraArgs: []string{"-target=mytarget"},
			expErr:    true,
		},
		{
			commentFlags: []string{"-target", "mytarget"},
			extraArgs:    []string{"-target=mytarget"},
			expErr:       true,
		},
		// Test false positives.
		{
			commentFlags: []string{"-targethahagotcha"},
			expErr:       false,
		},
		{
			extraArgs: []string{"-targethahagotcha"},
			expErr:    false,
		},
		{
			commentFlags: []string{"-targeted=weird"},
			expErr:       false,
		},
		{
			extraArgs: []string{"-targeted=weird"},
			expErr:    false,
		},
	}

	RegisterMockTestingT(t)

	for _, c := range cases {
		descrip := fmt.Sprintf("comments flags: %s extra args: %s",
			strings.Join(c.commentFlags, ", "), strings.Join(c.extraArgs, ", "))
		t.Run(descrip, func(t *testing.T) {
			tmpDir, cleanup := TempDir(t)
			defer cleanup()
			planPath := filepath.Join(tmpDir, "workspace.tfplan")
			err := ioutil.WriteFile(planPath, nil, 0644)
			Ok(t, err)
			terraform := mocks.NewMockClient()
			step := runtime.ApplyStepRunner{
				TerraformExecutor: terraform,
			}

			output, err := step.Run(models.ProjectCommandContext{
				Workspace:   "workspace",
				RepoRelDir:  ".",
				CommentArgs: c.commentFlags,
			}, c.extraArgs, tmpDir)
			Equals(t, "", output)
			if c.expErr {
				ErrEquals(t, "cannot run apply with -target because we are applying an already generated plan. Instead, run -target with atlantis plan", err)
			} else {
				Ok(t, err)
			}
		})
	}
}

// Test that apply works for remote applies.
func TestRun_RemoteApply_Success(t *testing.T) {
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	planPath := filepath.Join(tmpDir, "workspace.tfplan")
	planFileContents := `
An execution plan has been generated and is shown below.
Resource actions are indicated with the following symbols:
  - destroy

Terraform will perform the following actions:

  - null_resource.hi[1]


Plan: 0 to add, 0 to change, 1 to destroy.`
	err := ioutil.WriteFile(planPath, []byte("Atlantis: this plan was created by remote ops\n"+planFileContents), 0644)
	Ok(t, err)

	RegisterMockTestingT(t)
	outCh := make(chan terraform.Line)
	tfExec := &tfExecMock{OutCh: outCh}
	updater := mocks2.NewMockCommitStatusUpdater()
	o := runtime.ApplyStepRunner{
		AsyncTFExec:         tfExec,
		CommitStatusUpdater: updater,
	}
	tfVersion, _ := version.NewVersion("0.11.0")

	// Asynchronously start sending output on the channel.
	go func() {
		preConfirmOut := fmt.Sprintf(preConfirmOutFmt, planFileContents)
		for _, line := range strings.Split(preConfirmOut, "\n") {
			outCh <- terraform.Line{Line: line}
		}
		for _, line := range strings.Split(postConfirmOut, "\n") {
			outCh <- terraform.Line{Line: line}
		}
		close(outCh)
	}()

	ctx := models.ProjectCommandContext{
		Workspace:   "workspace",
		RepoRelDir:  ".",
		CommentArgs: []string{"comment", "args"},
		ProjectConfig: &valid.Project{
			TerraformVersion: tfVersion,
		},
	}
	output, err := o.Run(ctx, []string{"extra", "args"}, tmpDir)
	Ok(t, err)
	tfExec.PassedInputMutex.Lock()
	defer tfExec.PassedInputMutex.Unlock()
	Equals(t, "yes\n", tfExec.PassedInput)
	Equals(t, `
2019/02/27 21:47:36 [DEBUG] Using modified User-Agent: Terraform/0.11.11 TFE/d161c1b
null_resource.dir2[1]: Destroying... (ID: 8554368366766418126)
null_resource.dir2[1]: Destruction complete after 0s

Apply complete! Resources: 0 added, 0 changed, 1 destroyed.
`, output)

	Equals(t, []string{"apply", "-input=false", "-no-color", "extra", "args", "comment", "args"}, tfExec.CalledArgs)
	_, err = os.Stat(planPath)
	Assert(t, os.IsNotExist(err), "planfile should be deleted")

	// Check that the status was updated with the run url.
	runURL := "https://app.terraform.io/app/lkysow-enterprises/atlantis-tfe-test-dir2/runs/run-PiDsRYKGcerTttV2"
	updater.VerifyWasCalledOnce().UpdateProject(ctx, models.ApplyCommand, models.PendingCommitStatus, runURL)
	updater.VerifyWasCalledOnce().UpdateProject(ctx, models.ApplyCommand, models.SuccessCommitStatus, runURL)
}

// Test that if the plan is different, we error out.
func TestRun_RemoteApply_PlanChanged(t *testing.T) {
	tmpDir, cleanup := TempDir(t)
	defer cleanup()
	planPath := filepath.Join(tmpDir, "workspace.tfplan")
	planFileContents := `
An execution plan has been generated and is shown below.
Resource actions are indicated with the following symbols:
  - destroy

Terraform will perform the following actions:

  - null_resource.hi[1]


Plan: 0 to add, 0 to change, 1 to destroy.`
	err := ioutil.WriteFile(planPath, []byte("Atlantis: this plan was created by remote ops\n"+planFileContents), 0644)
	Ok(t, err)

	RegisterMockTestingT(t)
	outCh := make(chan terraform.Line)
	tfExec := &tfExecMock{OutCh: outCh}
	o := runtime.ApplyStepRunner{
		AsyncTFExec:         tfExec,
		CommitStatusUpdater: mocks2.NewMockCommitStatusUpdater(),
	}
	tfVersion, _ := version.NewVersion("0.11.0")

	// Asynchronously start sending output on the channel.
	go func() {
		preConfirmOut := fmt.Sprintf(preConfirmOutFmt, "not the expected plan!")
		for _, line := range strings.Split(preConfirmOut, "\n") {
			outCh <- terraform.Line{Line: line}
		}
		close(outCh)
	}()

	output, err := o.Run(models.ProjectCommandContext{
		Workspace:   "workspace",
		RepoRelDir:  ".",
		CommentArgs: []string{"comment", "args"},
		ProjectConfig: &valid.Project{
			TerraformVersion: tfVersion,
		},
	}, []string{"extra", "args"}, tmpDir)
	ErrEquals(t, `Plan generated during apply phase did not match plan generated during plan phase.
Aborting apply.

Expected Plan:

An execution plan has been generated and is shown below.
Resource actions are indicated with the following symbols:
  - destroy

Terraform will perform the following actions:

  - null_resource.hi[1]


Plan: 0 to add, 0 to change, 1 to destroy.
**************************************************

Actual Plan:

not the expected plan!
**************************************************

This likely occurred because someone applied a change to this state in-between
your plan and apply commands.
To resolve, re-run plan.`, err)
	Equals(t, "", output)
	tfExec.PassedInputMutex.Lock()
	defer tfExec.PassedInputMutex.Unlock()
	Equals(t, "no\n", tfExec.PassedInput)

	// Planfile should not be deleted.
	_, err = os.Stat(planPath)
	Ok(t, err)
}

type tfExecMock struct {
	// LinesToSend will be sent on the channel.
	LinesToSend string
	// OutCh can be set to your own channel so you can send whatever you want.
	// Won't be used if LinesToSend is set.
	OutCh chan terraform.Line
	// CalledArgs is what args we were called with.
	CalledArgs []string
	// PassedInput is set to the last string passed to our input channel.
	PassedInput      string
	PassedInputMutex sync.Mutex
}

func (t *tfExecMock) RunCommandAsync(log *logging.SimpleLogger, path string, args []string, v *version.Version, workspace string) (chan<- string, <-chan terraform.Line) {
	t.CalledArgs = args

	if t.OutCh == nil {
		t.OutCh = make(chan terraform.Line)
	}
	in := make(chan string)
	go func() {
		for inLine := range in {
			t.PassedInputMutex.Lock()
			t.PassedInput = inLine
			t.PassedInputMutex.Unlock()
		}
	}()

	go func() {
		if t.LinesToSend != "" {
			for _, line := range strings.Split(t.LinesToSend, "\n") {
				t.OutCh <- terraform.Line{Line: line}
			}
			close(t.OutCh)
			close(in)
		}
	}()
	return in, t.OutCh
}

var preConfirmOutFmt = `
Running apply in the remote backend. Output will stream here. Pressing Ctrl-C
will cancel the remote apply if its still pending. If the apply started it
will stop streaming the logs, but will not stop the apply running remotely.

Preparing the remote apply...

To view this run in a browser, visit:
https://app.terraform.io/app/lkysow-enterprises/atlantis-tfe-test-dir2/runs/run-PiDsRYKGcerTttV2

Waiting for the plan to start...

Terraform v0.11.11

Configuring remote state backend...
Initializing Terraform configuration...
2019/02/27 21:50:44 [DEBUG] Using modified User-Agent: Terraform/0.11.11 TFE/d161c1b
Refreshing Terraform state in-memory prior to plan...
The refreshed state will be used to calculate this plan, but will not be
persisted to local or remote state storage.

null_resource.dir2[0]: Refreshing state... (ID: 8492616078576984857)

------------------------------------------------------------------------
%s

Do you want to perform these actions in workspace "atlantis-tfe-test-dir2"?
  Terraform will perform the actions described above.
  Only 'yes' will be accepted to approve.`

var postConfirmOut = `
  Enter a value: 

2019/02/27 21:47:36 [DEBUG] Using modified User-Agent: Terraform/0.11.11 TFE/d161c1b
null_resource.dir2[1]: Destroying... (ID: 8554368366766418126)
null_resource.dir2[1]: Destruction complete after 0s

Apply complete! Resources: 0 added, 0 changed, 1 destroyed.
`
