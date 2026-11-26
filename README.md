# Chaos Launcher

Minecraft Java Edition için TikTok Chaos launcher projesi. Geliştirici: YσZSHI帝 ([@y0zshhiii](https://www.tiktok.com/@y0zshhiii)).

## Test sürümü v0.4.6

Bu sürüm Prism oturum açma penceresinin görünür açılmasını ve Windows bildirim alanı desteğini ekler.

- Hesabım > Microsoft ile giriş, resmi Prism penceresini normal modda açar. Mevcut Prism penceresini öne getirmeyi dener. Hesap ekleme ve lisans doğrulama Prism tarafından yapılır.
- Minecraft GLFW penceresi bu Chaos oturumuna ait olarak algılanırsa launcher bildirim alanına küçülür. X de boşta iken küçültür; simgeye tıklama pencereyi açar, sağ tık > Çıkış launcher işlemini sonlandırır.
- Oyun algılama yalnızca başlatma isteği sonrasında en fazla 15 dakika, 5 saniye aralıklarla yapılır; sürekli oyun taraması yoktur. Windows uçtan uca testi gereklidir.

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

GitHub Releases üzerinde `launcher-v0.4.6` gibi etiketle normal release oluşturun; EXE adı `TikTokChaosLauncher.exe` olmalıdır. Mod için `mod-vX.Y.Z` ve `tiktokchaos.jar`. GitHub SHA-256 asset digest gereklidir. Test sürümleri normal release olarak yayınlanırsa istemciler tarafından güncelleme olarak görülebilir.

## Lisans

Proje lisansı henüz belirlenmedi. Kaynakların görünür olması tek başına yeniden dağıtma veya değiştirme izni vermez. Ayrı ve değiştirilmemiş [Prism Launcher](https://github.com/PrismLauncher/PrismLauncher) kendi GPL-3.0 lisansıyla dağıtılır. Minecraft, Microsoft ve TikTok ile resmi bir ortaklık değildir.
