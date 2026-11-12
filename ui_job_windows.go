package main

import (
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// webviewMemoryArgs are appended to the WebView2 browser command line via the
// WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS environment variable (see main.go).
// SpareRendererForSitePerProcess avoids a pre-spawned spare renderer process;
// measured flags for the default set are recorded in PROGRESS.md.
const webviewMemoryArgs = "--disable-features=SpareRendererForSitePerProcess"

// A job owns the UI and its browser descendants, including processes that
// outlive the webview host briefly while flushing caches. Application writes
// remain in the resident and are not members of this job.
type uiJob struct{ handle windows.Handle }

func ownUIProcess(pid int) (*uiJob, error) {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err = windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(handle, process); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	return &uiJob{handle: handle}, nil
}

func (job *uiJob) stop() {
	_ = windows.TerminateJobObject(job.handle, 0)
	defer windows.CloseHandle(job.handle)
	type accounting struct {
		TotalUserTime, TotalKernelTime, PeriodUserTime, PeriodKernelTime int64
		PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var info accounting
		if err := windows.QueryInformationJobObject(job.handle, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil); err != nil || info.ActiveProcesses == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
}
