$ErrorActionPreference = "Stop"

$headers = @{Accept = 'application/vnd.github+json'}

$runs = Invoke-RestMethod -Uri 'https://api.github.com/repos/Know5/Clipbox/actions/runs?per_page=3' -Headers $headers

foreach ($run in $runs.workflow_runs) {
    Write-Host "====================================="
    Write-Host "Run #$($run.run_number) - $($run.head_commit.message.Substring(0, [Math]::Min(60,$run.head_commit.message.Length)))"
    Write-Host "Status: $($run.status)  Conclusion: $($run.conclusion)"

    $jobsUrl = $run.jobs_url
    $jobs = Invoke-RestMethod -Uri $jobsUrl -Headers $headers

    foreach ($job in $jobs.jobs) {
        Write-Host "  Job: $($job.name) - $($job.conclusion)"
        foreach ($step in $job.steps) {
            if ($step.conclusion -eq 'failure') {
                Write-Host "    FAILED: $($step.name)"
            }
        }
    }
    Write-Host ""
}
