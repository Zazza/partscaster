package studio

import (
	"context"
	"errors"
	"log"

	"partscaster/internal/yue"
)

// ApplyEdits — «применить всё» студии: трек пересобирается со всеми правками черновика, записи с целью сверки
// баланса (пресет звука, VerifyDb) сверяются по итогу и при промахе собираются заново (buildVerified) — одна
// операция общей очереди (вид rebuild). mixFile — сборка-прослушивание черновика теми же записями: первой сборки нет.
// Итог — один новый трек; в журнал — одна rebuild_sections с итоговыми записями и версия (рецепт повторит итог),
// черновик трека на воркере удаляется. Ошибка — версий нет, черновик остаётся.
func ApplyEdits(ctx context.Context, svc yue.Service, parentID int64, specs []SectionSpec, title, mixFile string) (int64, error) {
	if len(specs) == 0 {
		return 0, errors.New("нет правок для применения")
	}
	var b *verifiedBuild
	_, err := RunOp(ctx, svc, "rebuild", rebuildLabel(specs), parentID, func(ctx context.Context) (int64, error) {
		r, err := buildVerified(yue.WithoutJournal(ctx), svc, parentID, specs, mixFile, title,
			func(ctx context.Context, sp []SectionSpec) (string, error) {
				res, err := rebuildSections(ctx, svc, parentID, sp, "")
				if err != nil {
					return "", err
				}
				if res == nil || res.Variant == nil {
					return "", errors.New("пересборка не дала микса")
				}
				return res.Variant.File, nil
			})
		if err != nil {
			return 0, err
		}
		b = r
		// отмена принята в последний миг — версии отменённого итога в треке нет
		OnCanceled(ctx, func(c context.Context) {
			if _, derr := svc.DeleteJob(c, r.child); derr != nil {
				log.Printf("итог #%d отменён: версия #%d не удалена: %v", parentID, r.child, derr)
			}
		})
		return r.child, nil
	})
	if err != nil {
		return 0, err
	}
	recordRebuild(ctx, svc, parentID, b.specs, b.file)
	yue.Record(ctx, svc, parentID, yue.JobOp{Op: "variant_track", Params: yue.Params(map[string]any{"file": b.file,
		"title": title, "voice_src": 0}), Result: yue.JobResult(b.child)})
	// трек уже создан: сбой удаления черновика не делает применение неудачным — приложение
	// после успеха само очищает реестр трека и повторяет удаление при синхронизации
	_, _ = svc.DraftDelete(ctx, parentID)
	return b.child, nil
}
