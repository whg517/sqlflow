/**
 * Collapsible — shadcn/ui wrapper
 * Uses radix-ui Collapsible primitive
 */

import { Collapsible as CollapsiblePrimitive } from "radix-ui";

const Collapsible = CollapsiblePrimitive.Root;
const CollapsibleTrigger = CollapsiblePrimitive.Trigger;
const CollapsibleContent = CollapsiblePrimitive.Content;

export { Collapsible, CollapsibleTrigger, CollapsibleContent };
