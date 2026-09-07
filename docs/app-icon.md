# Иконка приложения

Редактируй **assets/app.png** (квадратный PNG с прозрачностью).
`tools/build.ps1` сам создаёт ICO размеров 16/24/32/48/64/128/256,
встраивает ресурс иконки в Windows EXE и удаляет промежуточный `.syso`.
PNG рядом с игрой используется для иконки окна; работает также при `go run ./cmd/game`.
Игровые спрайты и звук не вшиваются.

Генератор ресурсов: github.com/tc-hib/winres v0.3.1 (только инструмент сборки).
Рисунок: встроенный imagegen, 2026-09-07.
Промпт: Original medieval settlement application icon, compact pixel-art stone
watchtower with terracotta roof and golden wheat ear; transparent background,
no text or landscape, strong silhouette readable at 32 pixels.
