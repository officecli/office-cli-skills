import { motion } from "motion/react";
import { Terminal, Cpu, FileText, BarChart } from "lucide-react";

export default function Workflow() {
  const steps = [
    { icon: <Terminal className="text-primary" />, title: "Input", subtitle: "JSON / Prompt / Webhook" },
    { icon: <Cpu className="text-tertiary" />, title: "Process", subtitle: "Cloud Orchestration" },
    { icon: <FileText className="text-secondary" />, title: "Generate", subtitle: "High-Res Assets" },
    { icon: <BarChart className="text-primary-container" />, title: "Track", subtitle: "Usage & Logs" }
  ];

  return (
    <section className="py-24 bg-background border-y border-white/5">
      <div className="max-w-[1440px] mx-auto px-8 md:px-16 text-center">
        <h2 className="font-headline text-4xl md:text-5xl font-bold text-white mb-16 tracking-tight">The Kinetic Cycle</h2>
        <div className="flex flex-col md:flex-row items-center justify-between gap-8 relative">
          <div className="hidden md:block absolute top-1/2 left-0 w-full h-[1px] bg-gradient-to-r from-transparent via-outline-variant/30 to-transparent -translate-y-1/2 z-0"></div>
          
          {steps.map((step, index) => (
            <motion.div 
              key={index}
              initial={{ opacity: 0, y: 20 }}
              whileInView={{ opacity: 1, y: 0 }}
              viewport={{ once: true }}
              transition={{ delay: index * 0.1 }}
              className="relative z-10 flex flex-col items-center group w-full max-w-[280px]"
            >
              <div className="w-16 h-16 rounded-full bg-surface-high border border-outline-variant/20 flex items-center justify-center mb-6 group-hover:scale-110 transition-all duration-300">
                {step.icon}
              </div>
              <h4 className="font-headline text-lg font-bold text-white mb-2">{step.title}</h4>
              <p className="text-[10px] text-outline-variant font-headline uppercase tracking-widest">{step.subtitle}</p>
            </motion.div>
          ))}
        </div>
      </div>
    </section>
  );
}
