$c = Get-Process -Name clipbox -ErrorAction Stop

Write-Host ""
Write-Host "=== Task Manager View (Private = exclusive memory) ==="
Write-Host ""

$goWS   = [math]::Round($c.WorkingSet64/1MB, 1)
$goPriv = [math]::Round($c.PrivateMemorySize64/1MB, 1)
Write-Host ("clipbox.exe  WS={0}MB  Private={1}MB" -f $goWS, $goPriv)

$totalPriv = $c.PrivateMemorySize64
$totalWS   = $c.WorkingSet64

$kids = Get-CimInstance Win32_Process -Filter "ParentProcessId = $($c.Id)" -ErrorAction SilentlyContinue
foreach ($k in $kids) {
    $gp = Get-Process -Id $k.ProcessId -ErrorAction SilentlyContinue
    if (-not $gp) { continue }
    $ws   = [math]::Round($gp.WorkingSet64/1MB, 1)
    $priv = [math]::Round($gp.PrivateMemorySize64/1MB, 1)
    Write-Host ("+-- {0}  PID={1}  WS={2}MB  Private={3}MB" -f $gp.ProcessName, $gp.Id, $ws, $priv)
    $totalWS   += $gp.WorkingSet64
    $totalPriv += $gp.PrivateMemorySize64

    $gcs = Get-CimInstance Win32_Process -Filter "ParentProcessId = $($gp.Id)" -ErrorAction SilentlyContinue
    foreach ($g in $gcs) {
        $gg = Get-Process -Id $g.ProcessId -ErrorAction SilentlyContinue
        if (-not $gg) { continue }
        $ws2   = [math]::Round($gg.WorkingSet64/1MB, 1)
        $priv2 = [math]::Round($gg.PrivateMemorySize64/1MB, 1)
        Write-Host ("  +-- {0}  PID={1}  WS={2}MB  Private={3}MB" -f $gg.ProcessName, $gg.Id, $ws2, $priv2)
        $totalWS   += $gg.WorkingSet64
        $totalPriv += $gg.PrivateMemorySize64
    }
}

$goPrivOnly = [math]::Round($c.PrivateMemorySize64/1MB, 1)
$wvPrivOnly = [math]::Round(($totalPriv - $c.PrivateMemorySize64)/1MB, 1)
$totalPrivOnly = [math]::Round($totalPriv/1MB, 1)
$totalWSOnly = [math]::Round($totalWS/1MB, 1)

Write-Host ""
Write-Host "=========================================="
Write-Host ("  Go    Private:  {0,6:F1} MB" -f $goPrivOnly)
Write-Host ("  WebView Priv:  {0,6:F1} MB" -f $wvPrivOnly)
Write-Host ("  ------------------------------")
Write-Host ("  TOTAL Private:  {0,6:F1} MB  <-- Task Manager" -f $totalPrivOnly)
Write-Host ("  TOTAL WS:       {0,6:F1} MB  <-- incl shared" -f $totalWSOnly)
Write-Host "=========================================="
