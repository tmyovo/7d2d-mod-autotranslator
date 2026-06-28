package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) ChooseFolder() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择单个 7 Days to Die Mod 目录",
	})
}
