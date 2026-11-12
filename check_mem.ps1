param([int]$ProcessId = 0)
$ErrorActionPreference = 'Stop'
$rows = @(Get-CimInstance Win32_Process | Select-Object ProcessId, ParentProcessId, Name, CommandLine)
if ($ProcessId -eq 0) {
    $roots = @($rows | Where-Object { $_.Name -eq 'ClipBox.exe' -and $_.CommandLine -notmatch '(?:^|\s)--ui(?:\s|$)' })
    if ($roots.Count -ne 1) { throw '请使用 -ProcessId 指定一个正在运行的 ClipBox 后台进程。' }
    $ProcessId = [int]$roots[0].ProcessId
}
$family = [Collections.Generic.HashSet[int]]::new()
$family.Add($ProcessId) | Out-Null
do {
    $added = 0
    foreach ($row in $rows) {
        if ($family.Contains([int]$row.ParentProcessId) -and $family.Add([int]$row.ProcessId)) { $added++ }
    }
} while ($added -gt 0)
$privateWorkingSets = @{}
try {
    Get-CimInstance Win32_PerfFormattedData_PerfProc_Process | ForEach-Object {
        $privateWorkingSets[[int]$_.IDProcess] = [long]$_.WorkingSetPrivate
    }
} catch { Write-Verbose '系统性能计数器不可用，仍返回私有提交内存和工作集。' }
$metrics = @()
foreach ($familyId in $family) {
    $process = Get-Process -Id $familyId -ErrorAction SilentlyContinue
    if ($process) {
        $metrics += [pscustomobject]@{
            Name = $process.ProcessName
            PID = $familyId
            PrivateCommitMiB = [math]::Round($process.PrivateMemorySize64 / 1MB, 1)
            PrivateWorkingSetMiB = if ($privateWorkingSets.ContainsKey($familyId)) { [math]::Round($privateWorkingSets[$familyId] / 1MB, 1) } else { $null }
            WorkingSetMiB = [math]::Round($process.WorkingSet64 / 1MB, 1)
        }
    }
}
[pscustomobject]@{
    RootPID = $ProcessId
    ProcessCount = $metrics.Count
    TotalPrivateCommitMiB = [math]::Round(($metrics | Measure-Object PrivateCommitMiB -Sum).Sum, 1)
    TotalPrivateWorkingSetMiB = [math]::Round(($metrics | Measure-Object PrivateWorkingSetMiB -Sum).Sum, 1)
    Processes = $metrics
} | ConvertTo-Json -Depth 4
