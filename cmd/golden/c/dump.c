/* Calls every TA-Lib function through the abstract API over bars 0..N-1 with
 * default optional inputs and writes input.csv plus one <FUNCNAME>.csv per
 * function into the directory given as the only argument.
 *
 * Built and run by cmd/golden against the official static library. The input
 * binding must match the one cmd/gen writes into golden_gen_test.go. */
#include <math.h>
#include <stdarg.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "ta_libc.h"

#define N 400
#define MAX_FUNCS 1024
#define MAX_OUT 16

static double op[N], hi[N], lo[N], cl[N], vo[N], real1[N], periods[N];
static const char *names[MAX_FUNCS];
static int nnames;
static const char *flat[MAX_FUNCS];
static int nflat;
static const char *outDir;

static void die(const char *fmt, ...)
{
   va_list ap;
   va_start(ap, fmt);
   fputs("dump: ", stderr);
   vfprintf(stderr, fmt, ap);
   fputc('\n', stderr);
   va_end(ap);
   exit(1);
}

/* xorshift64*: integer-only, so the series is the same on every compiler and CPU. */
static uint64_t rng = 0x9E3779B97F4A7C15ULL;
static int64_t irand(int64_t lo_, int64_t hi_) /* [lo_, hi_] */
{
   rng ^= rng >> 12;
   rng ^= rng << 25;
   rng ^= rng >> 27;
   return lo_ + (int64_t)(((rng * 0x2545F4914F6CDD1DULL) >> 11) % (uint64_t)(hi_ - lo_ + 1));
}

/* Prices are whole units of 0.0001 divided by 10000: 4-decimal values, not dyadic
 * ones that would hide rounding, one correctly rounded division each. Close stays
 * mostly below 1, where ACOS and ASIN are defined, and crosses it, so that CEIL and
 * FLOOR vary. Each bar draws its candle shape, its opening gap and a direction from
 * short alternating runs, so that most candlestick patterns occur. */
static void make_data(void)
{
   int64_t o = 8500, c = 8500, c2 = 5500;
   int dir = 1, run = 0, i;
   for (i = 0; i < N; i++) {
      int64_t body, up, dn, po = o, top, bot, h, l;
      if (run-- == 0) {
         dir = -dir;
         run = (int)irand(0, 3);
      }
      if (c > 10200) dir = -1;
      else if (c < 6500) dir = 1;

      switch (irand(0, 9)) {
      case 0: o = (po > c ? po : c) + irand(1, 80); break;      /* gap up */
      case 1: o = (po < c ? po : c) - irand(1, 80); break;      /* gap down */
      case 2: o = po < c ? irand(po, c) : irand(c, po); break;  /* inside the last body */
      default: o = c + irand(-5, 5);
      }
      switch (irand(0, 6)) {
      case 0: body = irand(0, 2);     up = irand(5, 60);       dn = irand(5, 60);       break; /* doji */
      case 1: body = irand(8, 25);    up = irand(20, 70);      dn = irand(20, 70);      break; /* spinning top */
      case 2: body = irand(100, 250); up = irand(0, 15);       dn = irand(0, 15);       break; /* long body */
      case 3: body = irand(100, 250); up = 0;                  dn = 0;                  break; /* marubozu */
      case 4: body = irand(10, 40);   up = body * irand(2, 4); dn = irand(0, 3);        break; /* long upper shadow */
      case 5: body = irand(10, 40);   up = irand(0, 3);        dn = body * irand(2, 4); break; /* long lower shadow */
      default: body = irand(20, 100); up = irand(0, 40);       dn = irand(0, 40);
      }
      c = o + dir * body;
      top = o > c ? o : c;
      bot = o < c ? o : c;
      h = top + up;
      l = bot - dn;
      c2 += irand(-80, 80) - (c2 - 5500) / 100;
      if (l < 1000 || h > 19000) die("bar %d: a price left [0.1, 1.9]", i);
      if (c2 < 1000 || c2 > 9000) die("bar %d: real1 left [0.1, 0.9]", i);
      op[i] = (double)o / 10000.0;
      hi[i] = (double)h / 10000.0;
      lo[i] = (double)l / 10000.0;
      cl[i] = (double)c / 10000.0;
      vo[i] = (double)irand(1000, 99999);
      real1[i] = (double)c2 / 10000.0;
      periods[i] = (double)irand(2, 30);
   }
}

static FILE *create(const char *name)
{
   char path[4096];
   FILE *f;
   if (snprintf(path, sizeof path, "%s/%s.csv", outDir, name) >= (int)sizeof path)
      die("path too long for %s", name);
   if (!(f = fopen(path, "w"))) die("cannot create %s", path);
   return f;
}

static void finish(FILE *f, const char *name)
{
   if (ferror(f) | fclose(f)) die("write error on %s.csv", name);
}

static void putReal(FILE *f, double v)
{
   if (isnan(v)) fputs("NaN", f);
   else if (isinf(v)) fputs(v > 0 ? "+Inf" : "-Inf", f);
   else fprintf(f, "%.17g", v);
}

static void write_input(void)
{
   const double *cols[] = { op, hi, lo, cl, vo, real1, periods };
   FILE *f = create("input");
   int i;
   unsigned k;
   fputs("open,high,low,close,volume,real1,periods\n", f);
   for (i = 0; i < N; i++) {
      for (k = 0; k < sizeof cols / sizeof cols[0]; k++) {
         if (k) fputc(',', f);
         putReal(f, cols[k][i]);
      }
      fputc('\n', f);
   }
   finish(f, "input");
}

static void collect(const TA_FuncInfo *fi, void *opaque)
{
   (void)opaque;
   if (nnames == MAX_FUNCS) die("more than %d functions", MAX_FUNCS);
   names[nnames++] = fi->name;
}

static int cmp(const void *a, const void *b)
{
   return strcmp(*(const char *const *)a, *(const char *const *)b);
}

#define CHECK(call, what) \
   do { TA_RetCode rc_ = (call); if (rc_ != TA_SUCCESS) die("%s: %s failed, rc=%d", name, what, (int)rc_); } while (0)

static int same(double a, double b)
{
   return a == b || (isnan(a) && isnan(b));
}

static void dump_func(const char *name)
{
   static double outR[MAX_OUT][N];
   static int outI[MAX_OUT][N];
   const TA_OutputParameterInfo *oinfo[MAX_OUT];
   const TA_FuncHandle *h;
   const TA_FuncInfo *fi;
   TA_ParamHolder *ph;
   TA_Integer beg = -1, nb = -1;
   const TA_Integer allowedPrice = TA_IN_PRICE_OPEN | TA_IN_PRICE_HIGH | TA_IN_PRICE_LOW |
                                   TA_IN_PRICE_CLOSE | TA_IN_PRICE_VOLUME;
   unsigned k;
   int i;
   FILE *f;

   CHECK(TA_GetFuncHandle(name, &h), "TA_GetFuncHandle");
   CHECK(TA_GetFuncInfo(h, &fi), "TA_GetFuncInfo");
   if (fi->nbOutput == 0 || fi->nbOutput > MAX_OUT) die("%s: %u outputs", name, fi->nbOutput);
   CHECK(TA_ParamHolderAlloc(h, &ph), "TA_ParamHolderAlloc");

   for (k = 0; k < fi->nbInput; k++) {
      const TA_InputParameterInfo *ii;
      const char *pn;
      CHECK(TA_GetInputParameterInfo(h, k, &ii), "TA_GetInputParameterInfo");
      pn = ii->paramName ? ii->paramName : "(null)";
      switch (ii->type) {
      case TA_Input_Price:
         if (ii->flags & ~allowedPrice)
            die("%s: price input %s needs a component other than OHLCV (flags 0x%x)", name, pn, (unsigned)ii->flags);
         CHECK(TA_SetInputParamPricePtr(ph, k, op, hi, lo, cl, vo, NULL), "TA_SetInputParamPricePtr");
         break;
      case TA_Input_Real:
         if (strcmp(pn, "inReal") == 0 || strcmp(pn, "inReal0") == 0)
            CHECK(TA_SetInputParamRealPtr(ph, k, cl), "TA_SetInputParamRealPtr");
         else if (strcmp(pn, "inReal1") == 0)
            CHECK(TA_SetInputParamRealPtr(ph, k, real1), "TA_SetInputParamRealPtr");
         else if (strcmp(pn, "inPeriods") == 0)
            CHECK(TA_SetInputParamRealPtr(ph, k, periods), "TA_SetInputParamRealPtr");
         else
            die("%s: no binding for real input %s", name, pn);
         break;
      default:
         die("%s: no binding for input %s of type %d", name, pn, (int)ii->type);
      }
   }
   for (k = 0; k < fi->nbOutput; k++) {
      CHECK(TA_GetOutputParameterInfo(h, k, &oinfo[k]), "TA_GetOutputParameterInfo");
      switch (oinfo[k]->type) {
      case TA_Output_Real:
         CHECK(TA_SetOutputParamRealPtr(ph, k, outR[k]), "TA_SetOutputParamRealPtr");
         break;
      case TA_Output_Integer:
         CHECK(TA_SetOutputParamIntegerPtr(ph, k, outI[k]), "TA_SetOutputParamIntegerPtr");
         break;
      default:
         die("%s: output %s has unknown type %d", name, oinfo[k]->paramName, (int)oinfo[k]->type);
      }
   }
   CHECK(TA_CallFunc(ph, 0, N - 1, &beg, &nb), "TA_CallFunc");
   if (beg < 0 || nb < 0 || beg + nb > N) die("%s: outBegIdx %d, outNBElement %d out of range", name, (int)beg, (int)nb);

   /* Bars outside [beg, beg+nb) hold what the Go wrapper returns there. */
   f = create(name);
   for (k = 0; k < fi->nbOutput; k++) {
      if (k) fputc(',', f);
      fputs(oinfo[k]->paramName, f);
   }
   fputc('\n', f);
   for (i = 0; i < N; i++) {
      int in = i >= beg && i < beg + nb;
      for (k = 0; k < fi->nbOutput; k++) {
         if (k) fputc(',', f);
         if (oinfo[k]->type == TA_Output_Real) putReal(f, in ? outR[k][i - beg] : NAN);
         else fprintf(f, "%d", in ? outI[k][i - beg] : 0);
      }
      fputc('\n', f);
   }
   finish(f, name);
   CHECK(TA_ParamHolderFree(ph), "TA_ParamHolderFree");

   /* A column that never changes tests the wrapper little; reported, not an error. */
   for (k = 0; k < fi->nbOutput; k++) {
      for (i = 1; i < nb; i++) {
         if (oinfo[k]->type == TA_Output_Real ? !same(outR[k][i], outR[k][0]) : outI[k][i] != outI[k][0])
            break;
      }
      if (i >= nb) {
         flat[nflat++] = name;
         break;
      }
   }
}

int main(int argc, char **argv)
{
   int i;
   if (argc != 2) {
      fprintf(stderr, "usage: dump <out dir>\n");
      return 2;
   }
   outDir = argv[1];
   if (TA_Initialize() != TA_SUCCESS) die("TA_Initialize failed");
   make_data();
   write_input();
   if (TA_ForEachFunc(collect, NULL) != TA_SUCCESS) die("TA_ForEachFunc failed");
   if (nnames == 0) die("TA_ForEachFunc found no functions");
   qsort(names, (size_t)nnames, sizeof names[0], cmp);
   for (i = 0; i < nnames; i++) dump_func(names[i]);
   TA_Shutdown();
   printf("%d functions, %d bars\n", nnames, N);
   printf("%d with an output constant after the warmup:", nflat);
   for (i = 0; i < nflat; i++) printf(" %s", flat[i]);
   putchar('\n');
   return 0;
}
