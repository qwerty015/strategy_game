# Источники звука

Оба источника — CC0 (public domain), указывать авторство не обязано, но
оставлено для памяти проекта и на случай дальнейшего распространения.

## Звуковые эффекты (`sfx/`)

**[Kenney — Impact Sounds](https://kenney.nl/assets/impact-sounds)**,
`kenney_impact-sounds.zip`. Лицензия: CC0.

- `chop_000.ogg` .. `chop_004.ogg` — из `impactWood_medium_000..004.ogg`
  (топор лесоруба).
- `hammer_000.ogg` .. `hammer_004.ogg` — из
  `impactPlank_medium_000..004.ogg` (стройка).
- `mining_000.ogg` .. `mining_004.ogg` — из `impactMining_000..004.ogg`
  (каменоломня/добыча руды).

Взято только 15 из 130 файлов пака — остальные можно доиспользовать
позже (шаги, звук найма и т.п.).

## Фоновая музыка (`music/`)

**[OpenGameArt — Medieval: The Old Tower Inn](https://opengameart.org/content/medieval-the-old-tower-inn)**
by RandomMind, файл `Loop_The_Old_Tower_Inn.wav` → переименован в
`village.wav`. Лицензия: CC0. Использована именно loop-версия (не mp3 с
той же страницы) — она специально подготовлена под бесшовное зацикливание.

## Звуки построек, погоды и суток (`sfx/`, второй заход)

**[OpenGameArt — 100 CC0 SFX](https://opengameart.org/content/100-cc0-sfx)**
by rubberduck, `100-CC0-SFX_0.zip`. Лицензия: CC0. Один универсальный пак
закрыл сразу несколько построек — конкретного звука "коса по траве" или
"печь пекарни" в свободном доступе не нашлось, поэтому взяты наиболее
похожие по характеру категории из пака:

- `millwork_000..002.ogg` — из `machine_01..03.ogg` (мельница, механизмы).
- `squish_000..001.ogg` — из `plop_01..02.ogg` (винодельня, "шмяк").
- `oarsplash_000..001.ogg` — из `splash_01..02.ogg` (рыбацкая хижина).
- `tavernchatter_000..003.ogg` — из `dishes_01..04.ogg` (таверна, звон
  посуды вместо гула толпы — не нашли свободный гул толпы разумного
  размера).
- `cartcreak_000..003.ogg` — из `door_close_01..04.ogg` (склад, скрип
  двери от постоянных телег/слуг).
- `meatchop_000..003.ogg` — из `hit_01..04.ogg` (мясной цех).
- `saw_000..003.ogg` — из `wooden_01..03.ogg` и `wooded_box_open.ogg`
  (лесопилка/плотницкая мастерская).
- `forge_000..004.ogg` — из `metal_01..05.ogg` (кузница/литейная).
- `scythe_000..004.ogg` — из `tools_01..05.ogg` (ферма — замена косе, её
  в свободном доступе не нашлось).
- `bakery_000..001.ogg` — из `pot_01..02.ogg` (пекарня).

**[OpenGameArt — Solo Seagull Sound Effects](https://opengameart.org/content/solo-seagull-sound-effects)**
by (см. страницу пака), `Seagull Ambient 1..3.wav` → `seagull_000..002.ogg`
(перекодировано в ogg/vorbis). Лицензия: CC0. Играет, когда в кадре камеры
есть вода.

**[OpenGameArt — Crickets Ambient Noise - loopable](https://opengameart.org/content/crickets-ambient-noise-loopable)**,
`crickets.mp3` → `cricket_000.ogg` (перекодировано). Лицензия: CC0. Играет
ночью, не привязано к конкретной постройке.

**[OpenGameArt — Bird chirping sounds](https://opengameart.org/content/bird-chirping-sounds)**,
`birdchirping071414.wav` → `dayambience_000.ogg` (перекодировано).
Лицензия: CC0. Играет днём, не привязано к конкретной постройке.

**Freesound — хрюканье свиньи (свиноферма)**, три отдельных файла, лицензия
CC0 у каждого проверена индивидуально на странице (не у автора/пака в
целом — на freesound лицензия выставляется по каждому звуку):
- [gibarroule — Big pig loud oink](https://freesound.org/people/gibarroule/sounds/365135/) → `pigoink_000.ogg`
- [gibarroule — Pig Scream Loud Oink](https://freesound.org/people/gibarroule/sounds/365141/) → `pigoink_001.ogg`
- [Jofae — Angry Pig Oinking](https://freesound.org/people/Jofae/sounds/352698/) → `pigoink_002.ogg`

(Кандидат от Robinhood76 — `01161 boar oink 3.wav` — сознательно не взят:
у него лицензия CC-BY-NC, не подходит под конвенцию проекта "берём только
CC0".)

Все файлы этого раздела перекодированы в ogg/vorbis через `ffmpeg` для
единообразия с остальными `sfx/*.ogg` — оригинальные wav/mp3 не хранятся
в репозитории.

## Звук мира: шум воды (`sfx/`, третий заход)

**[OpenGameArt — 40 CC0 water / splash / slime SFX](https://opengameart.org/content/40-cc0-water-splash-slime-sfx)**
by rubberduck, `water-splash-slime-sfx.zip`. Лицензия: CC0. Уже готовые
ogg-файлы, без перекодирования.

- `waterwaves_000..002.ogg` — из `loop_water_01..03.ogg` (плеск/шум воды,
  играет вместе с чайками, когда в кадре камеры есть вода).

## Звук мира: ветер (`sfx/`, четвёртый заход)

Общий фоновый эмбиент, не привязан к камере или времени суток.

- [OpenGameArt — Wind Loop (whoosh)](https://opengameart.org/content/wind-whoosh-loop) →
  `wind_000.ogg`. Лицензия: CC0.
- [Freesound — felix.blume, Wind blowing in the countryside during spring, Tall Grass Prairie, Oklahoma, USA](https://freesound.org/people/felix.blume/sounds/215414/) →
  `wind_001.ogg` (обрезан до 15 сек). Лицензия: CC0 (проверена
  индивидуально на странице звука).
- [Freesound — felix.blume, Wind is blowing in the grass of a patagonian plain (Tierra del Fuego, Argentina)](https://freesound.org/people/felix.blume/sounds/139337/) →
  `wind_002.ogg` (обрезан до 15 сек). Лицензия: CC0 (проверена
  индивидуально).

(Кандидат "Wind Loop" на OpenGameArt — `wind-01_0.ogg` — сознательно не
взят: лицензия у него CC-BY, не CC0, не подходит под конвенцию проекта.)
