import { Component, input, output } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-confirm-dialog',
  standalone: true,
  imports: [CommonModule],
  template: `
    <div class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/50 backdrop-blur-sm transition-all animate-in fade-in duration-200">
      <div class="bg-white rounded-xl shadow-2xl max-w-md w-full overflow-hidden animate-in zoom-in-95 duration-200">
        <div class="p-6">
          <div class="flex items-center space-x-3 mb-4">
            <div class="w-10 h-10 rounded-full bg-red-100 flex items-center justify-center">
              <span class="material-symbols-outlined text-red-600">warning</span>
            </div>
            <h3 class="text-xl font-bold text-[#172B4D]">{{ title() }}</h3>
          </div>
          <p class="text-[#6B778C] leading-relaxed">{{ message() }}</p>
        </div>
        <div class="bg-[#F4F5F7] px-6 py-4 flex justify-end space-x-3">
          <button (click)="cancel.emit()" 
                  class="px-4 py-2 text-sm font-medium text-[#172B4D] hover:bg-[#EBECF0] rounded-md transition-colors">
            Cancel
          </button>
          <button (click)="confirm.emit()" 
                  class="px-4 py-2 text-sm font-medium text-white bg-red-600 hover:bg-red-700 rounded-md transition-shadow shadow-lg hover:shadow-red-200">
            Confirm
          </button>
        </div>
      </div>
    </div>
  `
})
export class ConfirmDialogComponent {
  title = input<string>('Confirm Action');
  message = input<string>('Are you sure you want to proceed?');
  confirm = output<void>();
  cancel = output<void>();
}
