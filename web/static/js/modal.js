/**
 * Modal Management for User Management UI
 * Handles opening/closing modals and integrates with HTMX events
 */

// Store the last focused element before opening modal
let lastFocusedElement = null;

/**
 * Show modal by removing 'hidden' class
 */
function showModal() {
  const modal = document.getElementById('userFormModal');
  if (modal) {
    // Store the currently focused element
    lastFocusedElement = document.activeElement;

    modal.classList.remove('hidden');
    modal.setAttribute('aria-hidden', 'false');

    // Focus on first input when modal opens (accessibility)
    setTimeout(() => {
      const firstInput = modal.querySelector('input:not([type="hidden"]), select, textarea');
      if (firstInput) {
        firstInput.focus();
      }

      // Setup focus trap
      setupFocusTrap(modal);
    }, 100);
  }
}

/**
 * Close modal and clear its content
 */
function closeModal() {
  const modal = document.getElementById('userFormModal');
  if (modal) {
    modal.classList.add('hidden');
    modal.setAttribute('aria-hidden', 'true');

    // Clear modal content to free memory
    const modalContent = modal.querySelector('.modal-content');
    if (modalContent) {
      modalContent.innerHTML = '';
    }

    // Restore focus to the element that opened the modal (accessibility)
    if (lastFocusedElement) {
      lastFocusedElement.focus();
      lastFocusedElement = null;
    }

    // Remove focus trap
    removeFocusTrap(modal);
  }
}

/**
 * Close modal when clicking outside the modal content
 */
function closeModalOnBackdropClick(event) {
  const modal = document.getElementById('userFormModal');
  if (modal && event.target === modal) {
    closeModal();
  }
}

/**
 * Setup focus trap to keep focus within modal
 */
function setupFocusTrap(modal) {
  modal.addEventListener('keydown', handleFocusTrap);
}

/**
 * Remove focus trap event listener
 */
function removeFocusTrap(modal) {
  modal.removeEventListener('keydown', handleFocusTrap);
}

/**
 * Handle focus trap - keep Tab navigation within modal
 */
function handleFocusTrap(event) {
  if (event.key !== 'Tab' && event.keyCode !== 9) {
    return;
  }

  const modal = event.currentTarget;
  const focusableElements = modal.querySelectorAll(
    'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), a[href], [tabindex]:not([tabindex="-1"])'
  );

  if (focusableElements.length === 0) {
    return;
  }

  const firstElement = focusableElements[0];
  const lastElement = focusableElements[focusableElements.length - 1];

  // Shift + Tab on first element: go to last element
  if (event.shiftKey && document.activeElement === firstElement) {
    event.preventDefault();
    lastElement.focus();
  }
  // Tab on last element: go to first element
  else if (!event.shiftKey && document.activeElement === lastElement) {
    event.preventDefault();
    firstElement.focus();
  }
}

/**
 * Close modal on ESC key press (accessibility)
 */
document.addEventListener('keydown', function(event) {
  if (event.key === 'Escape' || event.keyCode === 27) {
    closeModal();
  }
});

/**
 * HTMX Integration: Auto-close modal on successful form submission
 */
if (typeof htmx !== 'undefined') {
  // Listen for successful HTMX responses
  htmx.on('htmx:afterSwap', function(event) {
    // Check if the response was for the alerts container (success notification)
    if (event.detail.target && event.detail.target.id === 'alertsContainer') {
      // Check if the request was successful (2xx status code)
      const xhr = event.detail.xhr;
      if (xhr && xhr.status >= 200 && xhr.status < 300) {
        // Close modal after a short delay to allow user to see success message
        setTimeout(closeModal, 500);
      }
    }
  });

  // Listen for HTMX errors
  htmx.on('htmx:responseError', function(event) {
    console.error('HTMX Error:', event.detail);

    // For 401 errors (unauthorized), redirect to API keys page
    if (event.detail.xhr && event.detail.xhr.status === 401) {
      const alertsContainer = document.getElementById('alertsContainer');
      if (alertsContainer) {
        alertsContainer.innerHTML = `
          <div class="p-4 rounded-lg mb-4 flex items-start gap-3 bg-red-50 dark:bg-red-900/30 border border-red-200 dark:border-red-700">
            <svg class="w-5 h-5 text-red-600 dark:text-red-400 flex-shrink-0 mt-0.5" fill="currentColor" viewBox="0 0 20 20">
              <path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zM8.707 7.293a1 1 0 00-1.414 1.414L8.586 10l-1.293 1.293a1 1 0 101.414 1.414L10 11.414l1.293 1.293a1 1 0 001.414-1.414L11.414 10l1.293-1.293a1 1 0 00-1.414-1.414L10 8.586 8.707 7.293z" clip-rule="evenodd"></path>
            </svg>
            <div class="text-sm text-red-700 dark:text-red-300">
              Authentication failed. Please create an API key first.
            </div>
          </div>
        `;
      }

      // Redirect to API keys page after a delay
      setTimeout(() => {
        window.location.href = '/admin/apikeys';
      }, 2000);
    }
  });
}

/**
 * Initialize modal event listeners when DOM is ready
 */
document.addEventListener('DOMContentLoaded', function() {
  const modal = document.getElementById('userFormModal');
  if (modal) {
    // Click outside to close
    modal.addEventListener('click', closeModalOnBackdropClick);
  }
});
