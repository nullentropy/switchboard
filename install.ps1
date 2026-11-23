& {
	$ErrorActionPreference = 'Stop'
	$repo = if ($env:SWITCHBOARD_REPO) { $env:SWITCHBOARD_REPO } else { 'nullentropy/switchboard' }
	$dest = if ($env:SWITCHBOARD_DEST) { $env:SWITCHBOARD_DEST } else { Join-Path $env:LOCALAPPDATA 'Programs\Switchboard' }
	$asset = 'Switchboard-windows-amd64.zip'
	$url = $env:SWITCHBOARD_URL
	if (-not $url) {
		if ($repo -notmatch '^[^/]+/[^/]+$') {
			throw "install.ps1: SWITCHBOARD_REPO must be in owner/name form, such as nullentropy/switchboard, got: $repo"
		}
		$url = "https://github.com/$repo/releases/latest/download/$asset"
	}

	$work = Join-Path ([IO.Path]::GetTempPath()) ('switchboard-' + [guid]::NewGuid())
	New-Item -ItemType Directory -Path $work | Out-Null
	try {
		Write-Host "Downloading $url"
		$zip = Join-Path $work $asset
		$ProgressPreference = 'SilentlyContinue'
		Invoke-WebRequest -Uri $url -OutFile $zip -UseBasicParsing
		Expand-Archive -Path $zip -DestinationPath (Join-Path $work 'unzipped')
		$exe = Join-Path $work 'unzipped\Switchboard.exe'
		if (-not (Test-Path $exe)) {
			throw "install.ps1: $asset has no Switchboard.exe"
		}

		$target = Join-Path $dest 'Switchboard.exe'
		$running = @(Get-Process -Name Switchboard -ErrorAction SilentlyContinue | Where-Object { $_.Path -eq $target })
		if ($running.Count -gt 0) {
			Write-Host 'Quitting the running Switchboard'
			$running | ForEach-Object { [void]$_.CloseMainWindow() }
			$running | Wait-Process -Timeout 10 -ErrorAction SilentlyContinue
			$running | Where-Object { -not $_.HasExited } | Stop-Process -Force
		}

		New-Item -ItemType Directory -Force -Path $dest | Out-Null
		Copy-Item -Force $exe $target

		$link = Join-Path ([Environment]::GetFolderPath('Programs')) 'Switchboard.lnk'
		$shortcut = (New-Object -ComObject WScript.Shell).CreateShortcut($link)
		$shortcut.TargetPath = $target
		$shortcut.WorkingDirectory = $dest
		$shortcut.Save()
		Write-Host "Installed Switchboard to $target"
	} finally {
		Remove-Item -Recurse -Force -ErrorAction SilentlyContinue $work
	}
}
