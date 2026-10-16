package studio

import (
	"context"
	"errors"

	"partscaster/internal/yue"
)

// ApplyEdits — «применить всё» студии: трек пересобирается со всеми правками черновика
// (RebuildSections — с записью rebuild_sections в журнал, как обычно), микс становится
// одним новым треком (VariantToTrack — тоже в журнал, рецепт повторит итог), черновик
// трека на воркере удаляется. Ошибка шага — следующие не выполняются: черновик остаётся.
func ApplyEdits(ctx context.Context, svc yue.Service, parentID int64, specs []SectionSpec, title string) (int64, error) {
	if len(specs) == 0 {
		return 0, errors.New("нет правок для применения")
	}
	res, err := RebuildSections(ctx, svc, parentID, specs)
	if err != nil {
		return 0, err
	}
	if res == nil || res.Variant == nil {
		return 0, errors.New("пересборка не дала микса")
	}
	id, err := svc.VariantToTrack(ctx, parentID, res.Variant.File, title, 0)
	if err != nil {
		return 0, err
	}
	// трек уже создан: сбой удаления черновика не делает применение неудачным — приложение
	// после успеха само очищает реестр трека и повторяет удаление при синхронизации
	_, _ = svc.DraftDelete(ctx, parentID)
	return id, nil
}
