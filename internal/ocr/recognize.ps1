$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding($false)
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$null = [Windows.Storage.StorageFile, Windows.Storage, ContentType = WindowsRuntime]
$null = [Windows.Graphics.Imaging.BitmapDecoder, Windows.Graphics.Imaging, ContentType = WindowsRuntime]
$null = [Windows.Media.Ocr.OcrEngine, Windows.Foundation, ContentType = WindowsRuntime]
$null = [Windows.Globalization.Language, Windows.Globalization, ContentType = WindowsRuntime]

$asTask = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethod -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation`1'
} | Select-Object -First 1
function Await-WinRT($operation, [Type]$resultType) {
    $task = $asTask.MakeGenericMethod($resultType).Invoke($null, @($operation))
    $task.GetAwaiter().GetResult()
}

$stream = $null
$bitmap = $null
try {
    if ($env:CLIPBOX_OCR_MODE -eq 'languages') {
        $languages = @([Windows.Media.Ocr.OcrEngine]::AvailableRecognizerLanguages | ForEach-Object {
            @{ tag = $_.LanguageTag; name = $_.NativeName }
        })
        ConvertTo-Json -InputObject $languages -Compress
        exit 0
    }
    if ($env:CLIPBOX_OCR_LANGUAGE) {
        $language = New-Object Windows.Globalization.Language($env:CLIPBOX_OCR_LANGUAGE)
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromLanguage($language)
    } else {
        $engine = [Windows.Media.Ocr.OcrEngine]::TryCreateFromUserProfileLanguages()
    }
    if ($null -eq $engine) { throw 'OCR_LANGUAGE_UNAVAILABLE' }
    $file = Await-WinRT ([Windows.Storage.StorageFile]::GetFileFromPathAsync($env:CLIPBOX_OCR_IMAGE)) ([Windows.Storage.StorageFile])
    $stream = Await-WinRT ($file.OpenReadAsync()) ([Windows.Storage.Streams.IRandomAccessStreamWithContentType])
    $decoder = Await-WinRT ([Windows.Graphics.Imaging.BitmapDecoder]::CreateAsync($stream)) ([Windows.Graphics.Imaging.BitmapDecoder])
    $transform = New-Object Windows.Graphics.Imaging.BitmapTransform
    $limit = [Windows.Media.Ocr.OcrEngine]::MaxImageDimension
    $scaled = $false
    if ($decoder.PixelWidth -gt $limit -or $decoder.PixelHeight -gt $limit) {
        $scale = [Math]::Min($limit / $decoder.PixelWidth, $limit / $decoder.PixelHeight)
        $scaled = $true
        $transform.ScaledWidth = [uint32][Math]::Max(1, [Math]::Floor($decoder.PixelWidth * $scale))
        $transform.ScaledHeight = [uint32][Math]::Max(1, [Math]::Floor($decoder.PixelHeight * $scale))
    }
    $bitmap = Await-WinRT ($decoder.GetSoftwareBitmapAsync(
        [Windows.Graphics.Imaging.BitmapPixelFormat]::Bgra8,
        [Windows.Graphics.Imaging.BitmapAlphaMode]::Premultiplied,
        $transform,
        [Windows.Graphics.Imaging.ExifOrientationMode]::IgnoreExifOrientation,
        [Windows.Graphics.Imaging.ColorManagementMode]::DoNotColorManage
    )) ([Windows.Graphics.Imaging.SoftwareBitmap])
    $result = Await-WinRT ($engine.RecognizeAsync($bitmap)) ([Windows.Media.Ocr.OcrResult])
    $text = ($result.Lines | ForEach-Object { $_.Text }) -join "`n"
    @{ text = $text; language = $engine.RecognizerLanguage.LanguageTag; scaled = $scaled } | ConvertTo-Json -Compress
} catch {
    # Do not return file paths, script stacks or image data to the webview/log.
    $message = $_.Exception.ToString()
    if ($message.Contains('OCR_LANGUAGE_UNAVAILABLE')) { [Console]::Error.Write('OCR_LANGUAGE_UNAVAILABLE') }
    elseif ($message.Contains('OCR_IMAGE_TOO_LARGE')) { [Console]::Error.Write('OCR_IMAGE_TOO_LARGE') }
    else { [Console]::Error.Write('OCR_FAILED') }
    exit 1
} finally {
    if ($null -ne $bitmap) { $bitmap.Dispose() }
    if ($null -ne $stream) { $stream.Dispose() }
}
