#Requires -Version 5.1
#Requires -RunAsAdministrator
<#
.SYNOPSIS
    Install the sparkyctrl worker on Windows. The counterpart to deploy/install.sh.

.DESCRIPTION
    Downloads the prebuilt worker (or uses a local binary), installs it under Program Files,
    opens a Private-profile firewall rule for its port, and registers a Scheduled Task that
    runs it as SYSTEM at startup with auto-restart on crash. Re-run any time to update.
    Use -Uninstall to remove the task and firewall rule.

    WARNING: this installs a SYSTEM-level (root-equivalent) remote-code-execution daemon that
    listens on your network with token authentication by default. Trusted LAN only. Read the
    README before you run this. There is no undo beyond -Uninstall and regret.

.EXAMPLE
    .\install.ps1 -Fence C:\share -Start

.EXAMPLE
    .\install.ps1 -Uninstall
#>
[CmdletBinding()]
param(
    [string]$Addr        = "0.0.0.0:7766",
    [string]$Fence       = "",
    [string]$Audit       = "C:\ProgramData\sparkyctrl\audit.log",
    [string]$Token       = "",
    [switch]$NoAuth,
    [string]$Binary      = "",
    [string]$Version     = "latest",
    [string]$Repo        = "SimpleHonors/sparkyctrl",
    [string]$InstallDir  = "$Env:ProgramFiles\sparkyctrl",
    [string]$ServiceName = "sparkyctrl",
    [switch]$Start,
    [switch]$Uninstall,
    [switch]$NoFence
)

$ErrorActionPreference = "Stop"
# Invoke-WebRequest's progress bar makes downloads look frozen (and run far slower) on
# Windows PowerShell. Silence it so the install never appears to hang mid-download.
$ProgressPreference = "SilentlyContinue"
function Info($m) { Write-Host "==> $m" }
function New-WorkerToken {
    $bytes = New-Object byte[] 32
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try { $rng.GetBytes($bytes) } finally { $rng.Dispose() }
    ($bytes | ForEach-Object { $_.ToString("x2") }) -join ""
}
function Set-RestrictedAcl($Path) {
    if (-not (Test-Path -LiteralPath $Path)) { return }
    & icacls $Path /inheritance:r /grant:r "SYSTEM:F" "Administrators:F" | Out-Null
}
function Get-RelaunchArgs {
    $out = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $PSCommandPath)
    foreach ($entry in $PSBoundParameters.GetEnumerator()) {
        switch ($entry.Key) {
            "Addr"        { $out += @("-Addr", $entry.Value) }
            "Fence"       { if ($entry.Value) { $out += @("-Fence", $entry.Value) } }
            "Audit"       { $out += @("-Audit", $entry.Value) }
            "Token"       { if ($entry.Value) { $out += @("-Token", $entry.Value) } }
            "NoAuth"      { if ($entry.Value) { $out += "-NoAuth" } }
            "Binary"      { if ($entry.Value) { $out += @("-Binary", $entry.Value) } }
            "Version"     { $out += @("-Version", $entry.Value) }
            "Repo"        { $out += @("-Repo", $entry.Value) }
            "InstallDir"  { $out += @("-InstallDir", $entry.Value) }
            "ServiceName" { $out += @("-ServiceName", $entry.Value) }
            "Start"       { if ($entry.Value) { $out += "-Start" } }
            "Uninstall"   { if ($entry.Value) { $out += "-Uninstall" } }
            "NoFence"     { if ($entry.Value) { $out += "-NoFence" } }
        }
    }
    return $out
}

$port = ($Addr -split ":")[-1]
$authDir = Join-Path $Env:ProgramData "sparkyctrl"
$tokenPath = Join-Path $authDir "token.txt"

if ($Uninstall) {
    if (Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue) {
        Stop-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
        Unregister-ScheduledTask -TaskName $ServiceName -Confirm:$false
        Info "removed scheduled task '$ServiceName'"
    } else { Info "no scheduled task '$ServiceName' found" }
    Get-NetFirewallRule -DisplayName "sparkyctrl ($port)" -ErrorAction SilentlyContinue |
        Remove-NetFirewallRule -ErrorAction SilentlyContinue
    Remove-Item -LiteralPath $tokenPath -Force -ErrorAction SilentlyContinue
    if (Test-Path -LiteralPath $authDir) {
        try {
            Remove-Item -LiteralPath $authDir -Force -ErrorAction SilentlyContinue
        } catch { }
    }
    Info "left the binary + data in $InstallDir (delete manually if you want them gone)."
    return
}

# Fence decision must be explicit — no fence means FULL filesystem access, so never
# default to that silently. Prompt when interactive; require -Fence/-NoFence when input
# is redirected (e.g. irm | iex), where we cannot prompt.
if (-not $Fence -and -not $NoFence) {
    if ([Environment]::UserInteractive -and -not [Console]::IsInputRedirected) {
        while ($true) {
            Write-Host "Confine the worker's file operations to a directory (the `"fence`")?"
            Write-Host "  - enter a path to confine to it (recommended)"
            Write-Host "  - type 'none' for FULL filesystem access (dangerous)"
            $ans = Read-Host "fence path [or none]"
            if ($ans -eq 'none') { $NoFence = $true; break }
            elseif ($ans)        { $Fence = $ans;  break }
            else { Write-Host "Please enter a path or 'none'." }
        }
    } else {
        Write-Error "no terminal to prompt: pass -Fence <dir> to confine file operations, or -NoFence for full access"
        exit 1
    }
}

# If the service is already installed, stop it so its locked binary releases before we
# overwrite it — Windows won't replace a running .exe. Remember whether it was running so
# the update leaves it running afterward.
# If this script is itself running as a child of sparkyctrl (e.g. `sparkyctrl shell
# "powershell ... install.ps1"`), stopping the task kills our parent and takes us with
# it mid-install. Detect that case and re-spawn ourselves detached before the stop.
$wasRunning = $false
$existing = Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
if ($existing) {
    if ($existing.State -eq 'Running') { $wasRunning = $true }

    # Walk up the process tree looking for a sparkyctrl ancestor.
    $detach = $false
    $currentPid = $pid
    for ($up = 0; $up -lt 5; $up++) {
        $parent = Get-CimInstance Win32_Process -Filter "ProcessId=$currentPid" -ErrorAction SilentlyContinue
        if (-not $parent -or $parent.ParentProcessId -eq 0) { break }
        $pname = (Get-Process -Id $parent.ParentProcessId -ErrorAction SilentlyContinue).ProcessName
        if ($pname -eq "sparkyctrl") { $detach = $true; break }
        $currentPid = $parent.ParentProcessId
    }
    if ($detach) {
        # Re-spawn ourselves as an independent OS-level process that survives
        # the sparkyctrl task termination. Start-Process/Start-Job both die
        # with the parent; a detached PowerShell process does not.
        $relaunchArgs = Get-RelaunchArgs
        Start-Process -FilePath "powershell.exe" -ArgumentList $relaunchArgs -WindowStyle Hidden | Out-Null
        Info "running under sparkyctrl -- install detached to background"
        Info "sparkyctrl will restart in a few seconds"
        exit 0
    }

    Stop-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue
    Info "stopped existing '$ServiceName' to update its binary"
    for ($i = 0; $i -lt 20 -and (Get-ScheduledTask -TaskName $ServiceName -ErrorAction SilentlyContinue).State -eq 'Running'; $i++) {
        Start-Sleep -Milliseconds 250
    }
    Start-Sleep -Milliseconds 500
}

# 1. Resolve the binary. An explicit -Binary uses a local file; otherwise download the
#    release. We deliberately do NOT auto-pick a binary from the current directory: a
#    planted .\sparkyctrl.exe would otherwise be installed and run as SYSTEM.
$dest = Join-Path $InstallDir "sparkyctrl.exe"
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

if ($Binary) {
    $src = (Resolve-Path -LiteralPath $Binary).Path
    Copy-Item -LiteralPath $src -Destination $dest -Force
    Info "installed binary: $src -> $dest"
} else {
    if ($Version -eq "latest") {
        $url = "https://github.com/$Repo/releases/latest/download/sparkyctrl-windows-amd64.exe"
    } else {
        $url = "https://github.com/$Repo/releases/download/$Version/sparkyctrl-windows-amd64.exe"
    }
    Info "downloading $url"
    # Bypass any intermediate caches (GitHub CDN, transparent proxies) so a
    # re-run always fetches the current release, not a stale one.
    Invoke-WebRequest -Uri $url -OutFile $dest -UseBasicParsing `
        -Headers @{"Cache-Control"="no-cache"; "Pragma"="no-cache"}

    # Verify the binary actually works and report its version so the operator
    # can confirm the upgrade landed without a separate info call.
    try {
        $ver = & $dest @("--version") 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Error "downloaded binary failed to execute (exit code $LASTEXITCODE)"
            exit 1
        }
        Info "downloaded sparkyctrl version: $ver"
    } catch {
        Write-Error "downloaded binary does not appear to be executable: $_"
        exit 1
    }
}

# 2. Make sure the fence and audit-log locations exist.
if ($Fence) { New-Item -ItemType Directory -Force -Path $Fence | Out-Null }
New-Item -ItemType Directory -Force -Path (Split-Path $Audit) | Out-Null
# Create the audit file and make sure the SYSTEM service account can open it (the worker
# runs as SYSTEM and exits if it cannot open this file on startup). We do NOT lock it down
# like the token: the audit log is not a secret, and the Linux installer leaves it readable.
# /inheritance:e re-applies the parent directory's inherited ACEs (so admins / the interactive
# user keep access) and self-heals a stripped or empty audit-log ACL on reinstall; the explicit
# SYSTEM:F grant guarantees the service can always write it.
if (-not (Test-Path -LiteralPath $Audit)) { New-Item -ItemType File -Force -Path $Audit | Out-Null }
& icacls $Audit /inheritance:e /grant "*S-1-5-18:(F)" | Out-Null

# 3. Resolve the worker auth material.
if (-not $NoAuth) {
    New-Item -ItemType Directory -Force -Path $authDir | Out-Null
    if ($Token) {
        $workerToken = $Token
        $tokenSource = "override"
    } elseif (Test-Path -LiteralPath $tokenPath) {
        $workerToken = (Get-Content -LiteralPath $tokenPath -Raw).Trim()
        $tokenSource = "existing"
    } else {
        $workerToken = New-WorkerToken
        $tokenSource = "generated"
    }
    if (-not $workerToken) { throw "worker token was empty" }
    Set-Content -LiteralPath $tokenPath -NoNewline -Value $workerToken -Encoding ascii
    Set-RestrictedAcl $authDir
    Set-RestrictedAcl $tokenPath
    Info "auth token ($tokenSource): written to $tokenPath"
    Info "set `$Env:SPARKYCTRL_TOKEN from $tokenPath for client use (do NOT pass on the command line)"
}

# 4. Assemble the serve arguments.
$serveArgs = @("serve", "--addr", $Addr, "--audit", "`"$Audit`"")
if ($Fence) { $serveArgs += @("--fence", "`"$Fence`"") }
if ($NoAuth) { $serveArgs += @("--no-auth") }

# 5. Firewall: allow the port inbound on the Private profile only (never Public/internet).
Get-NetFirewallRule -DisplayName "sparkyctrl ($port)" -ErrorAction SilentlyContinue |
    Remove-NetFirewallRule -ErrorAction SilentlyContinue
New-NetFirewallRule -DisplayName "sparkyctrl ($port)" -Direction Inbound -Action Allow `
    -Protocol TCP -LocalPort $port -Profile Private -ErrorAction SilentlyContinue | Out-Null
Info "firewall: allowed inbound TCP $port on the Private profile"

# 6. Register a boot-start, auto-restarting Scheduled Task running as SYSTEM.
$action    = New-ScheduledTaskAction -Execute $dest -Argument ($serveArgs -join " ")
$trigger   = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings  = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
                -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
                -ExecutionTimeLimit ([TimeSpan]::Zero)

Register-ScheduledTask -TaskName $ServiceName -Action $action -Trigger $trigger `
    -Principal $principal -Settings $settings -Force | Out-Null
Info "registered scheduled task '$ServiceName' (runs as SYSTEM at startup, auto-restarts)"

if ($Start -or $wasRunning) {
    Start-ScheduledTask -TaskName $ServiceName
    Start-Sleep -Seconds 1
    Info "started: $((Get-ScheduledTask -TaskName $ServiceName).State)"
}

Info "done."
Write-Host ""
Write-Host "sparkyctrl is installed as SYSTEM and listening on $Addr." -ForegroundColor Yellow
Write-Host "This is a root-equivalent remote shell. Trusted LAN only. Remove with: .\install.ps1 -Uninstall" -ForegroundColor Yellow
