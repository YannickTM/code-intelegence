"use client";

import { Check } from "lucide-react";
import { cn } from "~/lib/utils";
import type { WizardStep } from "~/lib/wizard-state";

const STEPS: { num: WizardStep; label: string }[] = [
  { num: 1, label: "Project Details" },
  { num: 2, label: "SSH Key" },
  { num: 3, label: "Deploy Key" },
  { num: 4, label: "Done" },
];

export function StepIndicator({ currentStep }: { currentStep: WizardStep }) {
  return (
    <nav aria-label="Progress" className="flex items-center justify-between">
      {STEPS.map((step, idx) => {
        const isCompleted = currentStep > step.num;
        const isCurrent = currentStep === step.num;

        return (
          <div key={step.num} className="flex flex-1 items-center">
            {/* Circle + label */}
            <div className="flex flex-col items-center gap-1.5">
              <div
                className={cn(
                  "flex size-8 items-center justify-center rounded-full text-sm font-medium transition-colors",
                  isCompleted && "bg-primary text-primary-foreground",
                  isCurrent &&
                    "bg-primary text-primary-foreground ring-primary/30 ring-4",
                  !isCompleted &&
                    !isCurrent &&
                    "bg-muted text-muted-foreground",
                )}
              >
                {isCompleted ? (
                  <Check className="size-4" aria-hidden="true" />
                ) : (
                  step.num
                )}
              </div>
              <span
                className={cn(
                  "text-xs font-medium",
                  isCurrent ? "text-foreground" : "text-muted-foreground",
                )}
              >
                {step.label}
              </span>
            </div>

            {/* Connector line (not after last step) */}
            {idx < STEPS.length - 1 && (
              <div
                className={cn(
                  "mx-2 mb-5 h-0.5 flex-1",
                  currentStep > step.num ? "bg-primary" : "bg-border",
                )}
              />
            )}
          </div>
        );
      })}
    </nav>
  );
}
