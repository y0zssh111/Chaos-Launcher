# Chaos Launcher

Minecraft Java Edition için TikTok Chaos launcher projesi. Geliştirici: YσZSHI帝 ([@y0zshhiii](https://www.tiktok.com/@y0zshhiii)).

## Test sürümü v0.4.2

- Minecraft 1.21.11, Fabric 0.20.1 ve Java 21.
- Resmi, ayrı Prism Launcher üzerinden Microsoft hesabı bağlantısı ve oyun başlatma.
- Bu GitHub deposundan launcher ve Chaos mod güncellemelerini kontrol eder.
- Chaos mod JAR dosyası ve bağımlılıkları bu kaynak paketine dahil değildir.
- TikTok yayıncı doğrulaması sunucu ve TikTok onayı gerektirir; henüz canlı değildir.

## Derleme

Go 1.23 veya uyumlu sürümle:

```sh
go test ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags='-H windowsgui -s -w' -o TikTokChaosLauncher.exe .
```

Windows üzerinde gerçek hesap bağlantısı ve güncelleme akışı henüz uçtan uca doğrulanmamıştır. Ayrıntılar: [BENIOKU.txt](BENIOKU.txt).

## Yayınlama

GitHub Releases üzerinde `launcher-v0.4.2` gibi etiketle normal release oluşturun; EXE adı `TikTokChaosLauncher.exe` olmalıdır. Mod için `mod-vX.Y.Z` ve `tiktokchaos.jar`. GitHub SHA-256 asset digest gereklidir. Test sürümleri normal release olarak yayınlanırsa istemciler tarafından güncelleme olarak görülebilir.

## Lisans

Proje lisansı henüz belirlenmedi. Kaynakların görünür olması tek başına yeniden dağıtma veya değiştirme izni vermez. Ayrı ve değiştirilmemiş [Prism Launcher](https://github.com/PrismLauncher/PrismLauncher) kendi GPL-3.0 lisansıyla dağıtılır. Minecraft, Microsoft ve TikTok ile resmi bir ortaklık değildir.
