import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";

import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex min-h-9 items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-sm font-semibold shadow-none transition-all duration-150 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/20 active:scale-[0.96] active:brightness-95 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default:
          "border border-primary bg-primary text-white hover:bg-primary-hover",
        outline:
          "border border-line bg-white text-slate-700 hover:bg-slate-50 hover:text-primary",
        destructive:
          "border border-red-200 bg-white text-danger hover:bg-red-50",
        ghost:
          "border border-transparent bg-transparent text-slate-500 hover:bg-slate-50 hover:text-slate-900",
      },
      size: {
        default: "px-3.5 py-2",
        sm: "min-h-8 px-2.5 py-1.5 text-xs",
        lg: "min-h-10 px-5 py-2.5",
        icon: "h-8 w-8 min-h-8 p-0",
      },
    },
    defaultVariants: { variant: "outline", size: "default" },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Button.displayName = "Button";

export { buttonVariants };
